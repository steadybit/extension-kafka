/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kgo"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type ExecutionRunData struct {
	stopTicker            chan bool                  // stores the stop channels for each execution
	jobs                  chan time.Time             // stores the jobs for each execution
	tickers               *time.Ticker               // stores the tickers for each execution, to be able to stop them
	metrics               chan action_kit_api.Metric // stores the metrics for each execution
	requestCounter        atomic.Uint64              // stores the number of requests for each execution
	requestSuccessCounter atomic.Uint64              // stores the number of successful requests for each execution
}

var (
	ExecutionRunDataMap = sync.Map{} //make(map[uuid.UUID]*ExecutionRunData)
)

type HTTPCheckState struct {
	ExpectedStatusCodes      []int
	DelayBetweenRequestsInMS int64
	Timeout                  time.Time
	ResponsesContains        string
	SuccessRate              int
	ResponseTimeMode         string
	ResponseTime             *time.Duration
	MaxConcurrent            int
	NumberOfRequests         uint64
	ReadTimeout              time.Duration
	ExecutionID              uuid.UUID
	Body                     string
	URL                      url.URL
	Method                   string
	Headers                  map[string]string
	ConnectionTimeout        time.Duration
	FollowRedirects          bool
}

func prepare(request action_kit_api.PrepareActionRequestBody, state *KafkaBrokerAttackState, checkEnded func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool) (*action_kit_api.PrepareResult, error) {
	duration := extutil.ToInt64(request.Config["duration"])
	state.Timeout = time.Now().Add(time.Millisecond * time.Duration(duration))

	state.ResponsesContains = extutil.ToString(request.Config["responsesContains"])
	state.SuccessRate = extutil.ToInt(request.Config["successRate"])
	state.ResponseTimeMode = extutil.ToString(request.Config["responseTimeMode"])
	state.ResponseTime = extutil.Ptr(time.Duration(extutil.ToInt64(request.Config["responseTime"])) * time.Millisecond)
	state.MaxConcurrent = extutil.ToInt(request.Config["maxConcurrent"])
	state.NumberOfRequests = extutil.ToUInt64(request.Config["numberOfRequests"])
	state.ReadTimeout = time.Duration(extutil.ToInt64(request.Config["readTimeout"])) * time.Millisecond
	state.ExecutionID = request.ExecutionId
	state.Body = extutil.ToString(request.Config["body"])
	state.Method = extutil.ToString(request.Config["method"])
	state.ConnectionTimeout = time.Duration(extutil.ToInt64(request.Config["connectTimeout"])) * time.Millisecond
	state.FollowRedirects = extutil.ToBool(request.Config["followRedirects"])
	var err error
	state.Headers, err = extutil.ToKeyValue(request.Config, "headers")
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse headers")
		return nil, err
	}

	urlString, ok := request.Config["url"]
	if !ok {
		return nil, fmt.Errorf("URL is missing")
	}
	parsedUrl, err := url.Parse(extutil.ToString(urlString))
	if err != nil {
		log.Error().Err(err).Msg("URL could not be parsed missing")
		return nil, err
	}
	state.URL = *parsedUrl

	initExecutionRunData(state)
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load execution run data")
		return nil, err
	}

	// create worker pool
	for w := 1; w <= state.MaxConcurrent; w++ {
		go requestWorker(executionRunData, state, checkEnded)
	}
	return nil, nil
}

func loadExecutionRunData(executionID uuid.UUID) (*ExecutionRunData, error) {
	erd, ok := ExecutionRunDataMap.Load(executionID)
	if !ok {
		return nil, fmt.Errorf("failed to load execution run data")
	}
	executionRunData := erd.(*ExecutionRunData)
	return executionRunData, nil
}

func initExecutionRunData(state *KafkaBrokerAttackState) {
	saveExecutionRunData(state.ExecutionID, &ExecutionRunData{
		stopTicker:            make(chan bool),
		jobs:                  make(chan time.Time, state.MaxConcurrent),
		metrics:               make(chan action_kit_api.Metric, state.MaxConcurrent),
		requestCounter:        atomic.Uint64{},
		requestSuccessCounter: atomic.Uint64{},
	})
}

func saveExecutionRunData(executionID uuid.UUID, executionRunData *ExecutionRunData) {
	ExecutionRunDataMap.Store(executionID, executionRunData)
}

func createRecord(state *KafkaBrokerAttackState) (*kgo.Record, error) {
	// Generate a large payload (e.g., 1 MB)
	payload := make([]byte, state.RequestSizeMb*1024*1024)
	_, err := rand.Read(payload)
	if err != nil {
		return nil, err
	}

	record := &kgo.Record{
		Topic: state.Topic,
		Value: payload,
	}

	return record, err
}

func requestWorker(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState, checkEnded func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(config.Config.SeedBrokers),
		kgo.AllowAutoTopicCreation(),
		kgo.DefaultProduceTopic("steadybit-topic"),
		kgo.ClientID("steadybit"),
	}
	// TODO
	// - add TLS config if found
	// - add AWS config to client if found

	client, err := kgo.NewClient(opts...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create kafka client")
	}

	for range executionRunData.jobs {
		if !checkEnded(executionRunData, state) {
			var started = time.Now()

			rec, err := createRecord(state)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create record")
				now := time.Now()
				executionRunData.metrics <- action_kit_api.Metric{
					Metric: map[string]string{
						"topic":    rec.Topic,
						"producer": strconv.Itoa(int(rec.ProducerID)),
						"error":    err.Error(),
					},
					Name:      extutil.Ptr("response_time"),
					Value:     float64(now.Sub(started).Milliseconds()),
					Timestamp: now,
				}
				return
			}

			var producedRecord *kgo.Record
			producedRecord, err = client.ProduceSync(context.Background(), rec).First()

			if err != nil {
				log.Error().Err(err).Msg("Failed to produce record")
				now := time.Now()
				executionRunData.metrics <- action_kit_api.Metric{
					Metric: map[string]string{
						"topic":    rec.Topic,
						"producer": strconv.Itoa(int(rec.ProducerID)),
						"brokers":  config.Config.SeedBrokers,
						"error":    err.Error(),
					},
					Name:      extutil.Ptr("response_time"),
					Value:     float64(now.Sub(started).Milliseconds()),
					Timestamp: now,
				}
			} else {
				// Successfully produced the record
				recordProducerLatency := float64(producedRecord.Timestamp.Sub(started).Milliseconds())
				log.Debug().Msgf("Record have been produced at timestamp %s", producedRecord.Timestamp)
				metricMap := map[string]string{
					"topic":    rec.Topic,
					"producer": strconv.Itoa(int(rec.ProducerID)),
					"brokers":  config.Config.SeedBrokers,
					"error":    err.Error(),
				}
				executionRunData.requestCounter.Add(1)

				executionRunData.requestSuccessCounter.Add(1)

				metric := action_kit_api.Metric{
					Name:      extutil.Ptr("record_latency"),
					Metric:    metricMap,
					Value:     recordProducerLatency,
					Timestamp: producedRecord.Timestamp,
				}
				executionRunData.metrics <- metric
			}
		}
	}
}

func start(state *KafkaBrokerAttackState) {
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load execution run data")
	}
	executionRunData.tickers = time.NewTicker(time.Duration(state.DelayBetweenRequestsInMS) * time.Millisecond)
	executionRunData.stopTicker = make(chan bool)

	now := time.Now()
	log.Debug().Msgf("Schedule first record at %v", now)
	executionRunData.jobs <- now
	go func() {
		for {
			select {
			case <-executionRunData.stopTicker:
				log.Debug().Msg("Stop Request Scheduler")
				return
			case t := <-executionRunData.tickers.C:
				log.Debug().Msgf("Schedule Request at %v", t)
				executionRunData.jobs <- t
			}
		}
	}()
	ExecutionRunDataMap.Store(state.ExecutionID, executionRunData)
}

func retrieveLatestMetrics(metrics chan action_kit_api.Metric) []action_kit_api.Metric {

	statusMetrics := make([]action_kit_api.Metric, 0, len(metrics))
	for {
		select {
		case metric, ok := <-metrics:
			if ok {
				log.Debug().Msgf("Status Metric: %v", metric)
				statusMetrics = append(statusMetrics, metric)
			} else {
				log.Debug().Msg("Channel closed")
				return statusMetrics
			}
		default:
			log.Debug().Msg("No metrics available")
			return statusMetrics
		}
	}
}

func stop(state *KafkaBrokerAttackState) (*action_kit_api.StopResult, error) {
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Debug().Err(err).Msg("Execution run data not found, stop was already called")
		return nil, nil
	}
	stopTickers(executionRunData)

	latestMetrics := retrieveLatestMetrics(executionRunData.metrics)
	// calculate the success rate
	successRate := float64(executionRunData.requestSuccessCounter.Load()) / float64(executionRunData.requestCounter.Load()) * 100
	log.Debug().Msgf("Success Rate: %v%%", successRate)
	ExecutionRunDataMap.Delete(state.ExecutionID)
	if successRate < float64(state.SuccessRate) {
		log.Info().Msgf("Success Rate (%.2f%%) was below %v%%", successRate, state.SuccessRate)
		return extutil.Ptr(action_kit_api.StopResult{
			Metrics: extutil.Ptr(latestMetrics),
			Error: &action_kit_api.ActionKitError{
				Title:  fmt.Sprintf("Success Rate (%.2f%%) was below %v%%", successRate, state.SuccessRate),
				Status: extutil.Ptr(action_kit_api.Failed),
			},
		}), nil
	}
	log.Info().Msgf("Success Rate (%.2f%%) was above/equal %v%%", successRate, state.SuccessRate)
	return extutil.Ptr(action_kit_api.StopResult{
		Metrics: extutil.Ptr(latestMetrics),
	}), nil
}

func stopTickers(executionRunData *ExecutionRunData) {
	ticker := executionRunData.tickers
	if ticker != nil {
		ticker.Stop()
	}
	// non-blocking send
	select {
	case executionRunData.stopTicker <- true: // stop the ticker
		log.Trace().Msg("Stopped ticker")
	default:
		log.Debug().Msg("Ticker already stopped")
	}
}

/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kgo"
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

func prepare(request action_kit_api.PrepareActionRequestBody, state *KafkaBrokerAttackState, checkEnded func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool) (*action_kit_api.PrepareResult, error) {
	if len(request.Target.Attributes["kafka.topic.name"]) == 0 {
		return nil, fmt.Errorf("the target is missing the kafka.topic.name attribute")
	}
	state.Topic = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.name")[0]

	duration := extutil.ToInt64(request.Config["duration"])
	state.Timeout = time.Now().Add(time.Millisecond * time.Duration(duration))
	state.SuccessRate = extutil.ToInt(request.Config["successRate"])
	state.MaxConcurrent = extutil.ToInt(request.Config["maxConcurrent"])

	if state.MaxConcurrent == 0 {
		return nil, fmt.Errorf("max concurrent can't be zero")
	}
	state.NumberOfRecords = extutil.ToUInt64(request.Config["numberOfRecords"])
	state.RecordKey = extutil.ToString(request.Config["recordKey"])
	state.RecordValue = extutil.ToString(request.Config["recordValue"])
	state.ExecutionID = request.ExecutionId

	var err error
	if _, ok := request.Config["recordHeaders"]; ok {
		state.RecordHeaders, err = extutil.ToKeyValue(request.Config, "recordHeaders")
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse headers")
			return nil, err
		}
	}

	initExecutionRunData(state)
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load execution run data")
		return nil, err
	}

	// create worker pool
	for w := 1; w <= state.MaxConcurrent; w++ {
		go requestProducerWorker(executionRunData, state, checkEnded)
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

func createRecord(state *KafkaBrokerAttackState) *kgo.Record {
	record := kgo.KeyStringRecord(state.RecordKey, state.RecordValue)

	if state.RecordHeaders != nil {
		for k, v := range state.RecordHeaders {
			record.Headers = append(record.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}
	}

	return record
}

func requestProducerWorker(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState, checkEnded func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool) {
	client, err := createNewClient()
	if err != nil {
		log.Error().Err(err).Msg("Failed to create client")
	}
	for range executionRunData.jobs {
		if !checkEnded(executionRunData, state) {
			//var started = time.Now()
			rec := createRecord(state)

			//var producedRecord *kgo.Record
			_, err = client.ProduceSync(context.Background(), rec).First()
			executionRunData.requestCounter.Add(1)

			if err != nil {
				log.Error().Err(err).Msg("Failed to produce record")
				//now := time.Now()
				//executionRunData.metrics <- action_kit_api.Metric{
				//	Metric: map[string]string{
				//		"topic":    rec.Topic,
				//		"producer": strconv.Itoa(int(rec.ProducerID)),
				//		"brokers":  config.Config.SeedBrokers,
				//		"error":    err.Error(),
				//	},
				//	Name:      extutil.Ptr("producer_response_time"),
				//	Value:     float64(now.Sub(started).Milliseconds()),
				//	Timestamp: now,
				//}
			} else {
				// Successfully produced the record
				//recordProducerLatency := float64(producedRecord.Timestamp.Sub(started).Milliseconds())
				//metricMap := map[string]string{
				//	"topic":    rec.Topic,
				//	"producer": strconv.Itoa(int(rec.ProducerID)),
				//	"brokers":  config.Config.SeedBrokers,
				//	"error":    "",
				//}

				executionRunData.requestSuccessCounter.Add(1)

				//metric := action_kit_api.Metric{
				//	Name:      extutil.Ptr("record_latency"),
				//	Metric:    metricMap,
				//	Value:     recordProducerLatency,
				//	Timestamp: producedRecord.Timestamp,
				//}
				//executionRunData.metrics <- metric
			}
		}
	}
	err = client.Flush(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to flush")
	}
	defer client.Close()
}

//func requestConsumerWorker(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState, checkEnded func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool) {
//	opts := []kgo.Opt{
//		kgo.SeedBrokers(config.Config.SeedBrokers),
//		kgo.ConsumerGroup("steadybit-extension-kafka"),
//		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
//		kgo.ConsumeTopics(state.Topic),
//	}
//
//	client, err := kgo.NewClient(opts...)
//	if err != nil {
//		log.Error().Err(err).Msg("Failed to create client")
//	}
//	defer client.Close()
//
//	if !checkEnded(executionRunData, state) {
//		for range executionRunData.jobs {
//			// Poll for records
//			fetches := client.PollRecords(context.TODO(), 1)
//			if fetches.IsClientClosed() {
//				break
//			}
//
//			// Handle errors
//			if errs := fetches.Errors(); len(errs) > 0 {
//				for _, e := range errs {
//					log.Error().Err(e.Err).Msgf("Error consuming from topic %s partition %d", e.Topic, e.Partition)
//					return
//				}
//			}
//			executionRunData.requestCounter.Add(1)
//
//			// Process the records
//			fetches.EachRecord(func(record *kgo.Record) {
//				log.Debug().Msgf("Topic: %s, Partition: %d, Offset: %d\n",
//					record.Topic, record.Partition, record.Offset)
//				log.Debug().Msgf("Key: %s\n", string(record.Key))
//				log.Debug().Msgf("Value: %s\n", string(record.Value))
//				log.Debug().Msgf("Timestamp: %s\n", record.Timestamp)
//				log.Debug().Msgf("---")
//
//				executionRunData.requestSuccessCounter.Add(1)
//				return
//			})
//		}
//	}
//}

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
				log.Debug().Msg("Stop Record Scheduler")
				return
			case t := <-executionRunData.tickers.C:
				log.Debug().Msgf("Schedule Record at %v", t)
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

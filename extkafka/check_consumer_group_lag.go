/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"time"
)

type ConsumerGroupLagCheckAction struct{}

type ConsumerGroupLagCheckState struct {
	ConsumerGroupName string
	Topic             string
	End               time.Time
	AcceptableLag     int64
	StateCheckSuccess bool
}

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[ConsumerGroupLagCheckState]           = (*ConsumerGroupLagCheckAction)(nil)
	_ action_kit_sdk.ActionWithStatus[ConsumerGroupLagCheckState] = (*ConsumerGroupLagCheckAction)(nil)
)

func NewConsumerGroupLagCheckAction() action_kit_sdk.Action[ConsumerGroupLagCheckState] {
	return &ConsumerGroupLagCheckAction{}
}

func (m *ConsumerGroupLagCheckAction) NewEmptyState() ConsumerGroupLagCheckState {
	return ConsumerGroupLagCheckState{}
}

func (m *ConsumerGroupLagCheckAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check-lag", kafkaConsumerTargetId),
		Label:       "Check Topic Lag",
		Description: "check if the consumer group have lags on a topic",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType:          kafkaConsumerTargetId,
			QuantityRestriction: extutil.Ptr(action_kit_api.All),
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "default",
					Description: extutil.Ptr("Find consumer group by name"),
					Query:       "kafka.consumer-group.name=\"\"",
				},
			}),
		}),
		Technology:  extutil.Ptr("Kafka"),
		Kind:        action_kit_api.Check,
		TimeControl: action_kit_api.TimeControlInternal,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:         "duration",
				Label:        "Duration",
				Description:  extutil.Ptr(""),
				Type:         action_kit_api.Duration,
				DefaultValue: extutil.Ptr("30s"),
				Order:        extutil.Ptr(1),
				Required:     extutil.Ptr(true),
			},
			{
				Name:        "topic",
				Label:       "Topic to track lag",
				Description: extutil.Ptr("One topic to track lags"),
				Type:        action_kit_api.String,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.consumer-group.topics",
					},
				}),
				Order: extutil.Ptr(2),
			},
			{
				Name:         "acceptableLag",
				Label:        "Lag alert threshold",
				Description:  extutil.Ptr("How much lag is acceptable for this topic"),
				Type:         action_kit_api.Integer,
				Required:     extutil.Ptr(true),
				DefaultValue: extutil.Ptr("10"),
				Order:        extutil.Ptr(3),
			},
		},
		Widgets: extutil.Ptr([]action_kit_api.Widget{
			action_kit_api.StateOverTimeWidget{
				Type:  action_kit_api.ComSteadybitWidgetStateOverTime,
				Title: "Consumer Group Topic Lag",
				Identity: action_kit_api.StateOverTimeWidgetIdentityConfig{
					From: "kafka.consumer-group.name",
				},
				Label: action_kit_api.StateOverTimeWidgetLabelConfig{
					From: "kafka.consumer-group.name",
				},
				State: action_kit_api.StateOverTimeWidgetStateConfig{
					From: "state",
				},
				Tooltip: action_kit_api.StateOverTimeWidgetTooltipConfig{
					From: "tooltip",
				},
				Url: extutil.Ptr(action_kit_api.StateOverTimeWidgetUrlConfig{
					From: extutil.Ptr("url"),
				}),
				Value: extutil.Ptr(action_kit_api.StateOverTimeWidgetValueConfig{
					Hide: extutil.Ptr(true),
				}),
			},
		}),
		Status: extutil.Ptr(action_kit_api.MutatingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr("1s"),
		}),
	}
}

func (m *ConsumerGroupLagCheckAction) Prepare(_ context.Context, state *ConsumerGroupLagCheckState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	consumerGroupName := extutil.MustHaveValue(request.Target.Attributes, "kafka.consumer-group.name")
	state.Topic = extutil.ToString(request.Config["topic"])
	state.AcceptableLag = extutil.ToInt64(request.Config["acceptableLag"])

	duration := request.Config["duration"].(float64)
	end := time.Now().Add(time.Millisecond * time.Duration(duration))

	state.ConsumerGroupName = consumerGroupName[0]
	state.End = end

	return nil, nil
}

func (m *ConsumerGroupLagCheckAction) Start(_ context.Context, _ *ConsumerGroupLagCheckState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *ConsumerGroupLagCheckAction) Status(ctx context.Context, state *ConsumerGroupLagCheckState) (*action_kit_api.StatusResult, error) {
	return ConsumerGroupLagCheckStatus(ctx, state)
}

func ConsumerGroupLagCheckStatus(ctx context.Context, state *ConsumerGroupLagCheckState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	opts := []kgo.Opt{
		kgo.SeedBrokers(config.Config.SeedBrokers),
		kgo.ClientID("steadybit"),
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	adminClient := kadm.NewClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	lags, err := adminClient.Lag(ctx, state.ConsumerGroupName)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve consumer groups from Kafka for name %s. Full response: %v", state.ConsumerGroupName), err))
	}

	var groupLag kadm.DescribedGroupLag
	if len(lags.Sorted()) == 0 {
		log.Err(err).Msgf("No lags for consumer group with that name %s.", state.ConsumerGroupName)
	} else if len(lags.Sorted()) > 1 {
		log.Err(err).Msgf("More than 1 lag description for consumer group with that name %s.", state.ConsumerGroupName)
	} else {
		groupLag = lags.Sorted()[0]
	}

	if groupLag.FetchErr != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Error when fetching or describing the consumer group %s: %s", state.ConsumerGroupName, groupLag.FetchErr.Error()), groupLag.FetchErr))
	}
	if groupLag.DescribeErr != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Error when fetching or describing the consumer group %s: %s", state.ConsumerGroupName, groupLag.DescribeErr.Error()), groupLag.DescribeErr))

	}
	topicLag := groupLag.Lag.TotalByTopic()[state.Topic].Lag

	completed := now.After(state.End)
	var checkError *action_kit_api.ActionKitError
	if topicLag < state.AcceptableLag {
		state.StateCheckSuccess = true
	}
	if completed && !state.StateCheckSuccess {
		checkError = extutil.Ptr(action_kit_api.ActionKitError{
			Title: fmt.Sprintf("Consumer Group Lag was higher once than acceptable threshold  '%d'.",
				state.AcceptableLag),
			Status: extutil.Ptr(action_kit_api.Failed),
		})
	}

	metrics := []action_kit_api.Metric{
		*toMetric(topicLag, state, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   extutil.Ptr(metrics),
	}, nil
}

func toMetric(topicLag int64, stateGroupLag *ConsumerGroupLagCheckState, now time.Time) *action_kit_api.Metric {
	var tooltip string
	var state string

	if topicLag > stateGroupLag.AcceptableLag {
		state = "danger"
	} else {
		state = "success"
	}

	tooltip = fmt.Sprintf("consumer %s lag for topic %s is %d", stateGroupLag.ConsumerGroupName, stateGroupLag.Topic, topicLag)

	return extutil.Ptr(action_kit_api.Metric{
		Name: extutil.Ptr("kafka_consumer_group_state"),
		Metric: map[string]string{
			"kafka.topic.name":          stateGroupLag.Topic,
			"kafka.consumer-group.name": stateGroupLag.ConsumerGroupName,
			"url":                       "",
			"state":                     state,
			"tooltip":                   tooltip,
		},
		Timestamp: now,
		Value:     0,
	})
}

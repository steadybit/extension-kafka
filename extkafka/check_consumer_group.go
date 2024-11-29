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
	"slices"
	"strings"
	"time"
)

type ConsumerGroupCheckAction struct{}

type ConsumerGroupCheckState struct {
	ConsumerGroupName string
	TopicName         string
	End               time.Time
	ExpectedState     []string
	StateCheckMode    string
	StateCheckSuccess bool
	BrokerHosts       []string
}

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[ConsumerGroupCheckState]           = (*ConsumerGroupCheckAction)(nil)
	_ action_kit_sdk.ActionWithStatus[ConsumerGroupCheckState] = (*ConsumerGroupCheckAction)(nil)
)

func NewConsumerGroupCheckAction() action_kit_sdk.Action[ConsumerGroupCheckState] {
	return &ConsumerGroupCheckAction{}
}

func (m *ConsumerGroupCheckAction) NewEmptyState() ConsumerGroupCheckState {
	return ConsumerGroupCheckState{}
}

func (m *ConsumerGroupCheckAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check", kafkaConsumerTargetId),
		Label:       "Check Consumer State",
		Description: "Check the consumer state",
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
		Category:    extutil.Ptr("Kafka"),
		Kind:        action_kit_api.Check,
		TimeControl: action_kit_api.TimeControlInternal,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:         "duration",
				Label:        "Duration",
				Description:  extutil.Ptr(""),
				Type:         action_kit_api.Duration,
				DefaultValue: extutil.Ptr("30s"),
				Required:     extutil.Ptr(true),
			},
			{
				Name:        "expectedStateList",
				Label:       "Expected State List",
				Description: extutil.Ptr(""),
				Type:        action_kit_api.StringArray,
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "Unknown",
						Value: "Unknown",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "PreparingRebalance",
						Value: "PreparingRebalance",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "CompletingRebalance",
						Value: "CompletingRebalance",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Stable",
						Value: "Stable",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Dead",
						Value: "Dead",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Empty",
						Value: "Empty",
					},
				}),
				Required: extutil.Ptr(false),
			},
			{
				Name:         "stateCheckMode",
				Label:        "State Check Mode",
				Description:  extutil.Ptr("How often should the state be checked ?"),
				Type:         action_kit_api.String,
				DefaultValue: extutil.Ptr(stateCheckModeAllTheTime),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "All the time",
						Value: stateCheckModeAllTheTime,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "At least once",
						Value: stateCheckModeAtLeastOnce,
					},
				}),
				Required: extutil.Ptr(true),
			},
		},
		Widgets: extutil.Ptr([]action_kit_api.Widget{
			action_kit_api.StateOverTimeWidget{
				Type:  action_kit_api.ComSteadybitWidgetStateOverTime,
				Title: "Kafka Consumer Group State",
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

func (m *ConsumerGroupCheckAction) Prepare(_ context.Context, state *ConsumerGroupCheckState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	if len(request.Target.Attributes["kafka.consumer-group.name"]) == 0 {
		return nil, fmt.Errorf("the target is missing the kafka.consumer-group.name attribute")
	}

	duration := request.Config["duration"].(float64)
	end := time.Now().Add(time.Millisecond * time.Duration(duration))

	var expectedState []string
	if request.Config["expectedStateList"] != nil {
		expectedState = extutil.ToStringArray(request.Config["expectedStateList"])
	}

	var stateCheckMode string
	if request.Config["stateCheckMode"] != nil {
		stateCheckMode = fmt.Sprintf("%v", request.Config["stateCheckMode"])
	}

	state.ConsumerGroupName = request.Target.Attributes["kafka.consumer-group.name"][0]
	state.End = end
	state.ExpectedState = expectedState
	state.StateCheckMode = stateCheckMode
	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")

	return nil, nil
}

func (m *ConsumerGroupCheckAction) Start(_ context.Context, _ *ConsumerGroupCheckState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *ConsumerGroupCheckAction) Status(ctx context.Context, state *ConsumerGroupCheckState) (*action_kit_api.StatusResult, error) {
	return ConsumerGroupCheckStatus(ctx, state)
}

func ConsumerGroupCheckStatus(ctx context.Context, state *ConsumerGroupCheckState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	client, err := createNewAdminClient(state.BrokerHosts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	groups, err := client.DescribeGroups(ctx, state.ConsumerGroupName)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve consumer groups from Kafka for name %s. Full response: %v", state.ConsumerGroupName, err), err))
	}

	var group kadm.DescribedGroup
	if len(groups.Sorted()) == 0 {
		log.Err(err).Msgf("No consumer group with that name %s.", state.ConsumerGroupName)
	} else if len(groups.Sorted()) > 1 {
		log.Err(err).Msgf("More than 1 consumer group with that name %s.", state.ConsumerGroupName)
	} else {
		group = groups.Sorted()[0]
	}

	completed := now.After(state.End)
	var checkError *action_kit_api.ActionKitError

	if len(state.ExpectedState) > 0 {
		if state.StateCheckMode == stateCheckModeAllTheTime {
			if !slices.Contains(state.ExpectedState, group.State) {
				checkError = extutil.Ptr(action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Consumer Group '%s' has state '%s' whereas '%s' is expected.",
						group.Group,
						group.State,
						state.ExpectedState),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		} else if state.StateCheckMode == stateCheckModeAtLeastOnce {
			if slices.Contains(state.ExpectedState, group.State) {
				state.StateCheckSuccess = true
			}
			if completed && !state.StateCheckSuccess {
				checkError = extutil.Ptr(action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Consumer Group '%s' didn't have status '%s' at least once.",
						group.Group,
						state.ExpectedState),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		}
	}

	metrics := []action_kit_api.Metric{
		*toConsumerGroupMetric(group, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   extutil.Ptr(metrics),
	}, nil
}

func toConsumerGroupMetric(group kadm.DescribedGroup, now time.Time) *action_kit_api.Metric {
	var tooltip string
	var state string

	tooltip = fmt.Sprintf("Consumer group state is: %s", group.State)
	if group.State == "Stable" {
		state = "success"
	} else if group.State == "Empty" {
		state = "warn"
	} else if group.State == "PreparingRebalance" {
		state = "warn"
	} else if group.State == "Dead" {
		state = "danger"
	}

	return extutil.Ptr(action_kit_api.Metric{
		Name: extutil.Ptr("kafka_consumer_group_state"),
		Metric: map[string]string{
			"kafka.consumer-group.name": group.Group,
			"url":                       "",
			"state":                     state,
			"tooltip":                   tooltip,
		},
		Timestamp: now,
		Value:     0,
	})
}

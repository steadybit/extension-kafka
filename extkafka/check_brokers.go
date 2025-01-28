/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/extension-kafka/config"
	"strings"

	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"

	"slices"
	"sort"
	"time"
)

type BrokersCheckAction struct{}

type BrokersCheckState struct {
	PreviousController int32
	BrokerNodes        []int32
	End                time.Time
	ExpectedChanges    []string
	StateCheckMode     string
	StateCheckSuccess  bool
	BrokerHosts        []string
}

const (
	BrokerControllerChanged = "kafka controller changed"
	BrokerDowntime          = "kafka broker with downtime"
	MetricID                = "Broker Activity"
)

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[BrokersCheckState]           = (*BrokersCheckAction)(nil)
	_ action_kit_sdk.ActionWithStatus[BrokersCheckState] = (*BrokersCheckAction)(nil)
)

func NewBrokersCheckAction() action_kit_sdk.Action[BrokersCheckState] {
	return &BrokersCheckAction{}
}

func (m *BrokersCheckAction) NewEmptyState() BrokersCheckState {
	return BrokersCheckState{}
}

func (m *BrokersCheckAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check", kafkaBrokerTargetId),
		Label:       "Check Brokers",
		Description: "Check activity of brokers.",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
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
				Name:        "expectedChanges",
				Label:       "Expected Changes",
				Description: extutil.Ptr(""),
				Type:        action_kit_api.StringArray,
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "New Controller Elected",
						Value: BrokerControllerChanged,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Broker downtime",
						Value: BrokerDowntime,
					},
				}),
				Required: extutil.Ptr(false),
			},
			{
				Name:         "changeCheckMode",
				Label:        "Change Check Mode",
				Description:  extutil.Ptr("How do we check the change of the broker?"),
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
				Title: "Kafka Broker Changes",
				Identity: action_kit_api.StateOverTimeWidgetIdentityConfig{
					From: "metric.id",
				},
				Label: action_kit_api.StateOverTimeWidgetLabelConfig{
					From: "metric.id",
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
			CallInterval: extutil.Ptr("2s"),
		}),
	}
}

func (m *BrokersCheckAction) Prepare(ctx context.Context, state *BrokersCheckState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	duration := request.Config["duration"].(float64)
	end := time.Now().Add(time.Millisecond * time.Duration(duration))

	var expectedState []string
	if request.Config["expectedChanges"] != nil {
		expectedState = extutil.ToStringArray(request.Config["expectedChanges"])
	}

	var stateCheckMode string
	if request.Config["changeCheckMode"] != nil {
		stateCheckMode = fmt.Sprintf("%v", request.Config["changeCheckMode"])
	}

	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")

	client, err := createNewAdminClient(state.BrokerHosts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve brokers from Kafka. Full response: %v", err), err))
	}

	state.End = end
	state.ExpectedChanges = expectedState
	state.StateCheckMode = stateCheckMode
	state.PreviousController = metadata.Controller
	state.BrokerNodes = metadata.Brokers.NodeIDs()

	return nil, nil
}

func (m *BrokersCheckAction) Start(_ context.Context, _ *BrokersCheckState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *BrokersCheckAction) Status(ctx context.Context, state *BrokersCheckState) (*action_kit_api.StatusResult, error) {
	return BrokerCheckStatus(ctx, state)
}

func BrokerCheckStatus(ctx context.Context, state *BrokersCheckState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	client, err := createNewAdminClient(state.BrokerHosts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve brokers from Kafka. Full response: %v", err), err))
	}

	// Check for changes
	changes := make(map[string][]int32)

	if metadata.Controller != state.PreviousController {
		state.PreviousController = metadata.Controller
		changes[BrokerControllerChanged] = []int32{metadata.Controller}
	}

	if !areSlicesEqualUnordered(state.BrokerNodes, metadata.Brokers.NodeIDs()) {
		changes[BrokerDowntime] = findMissingElements(state.BrokerNodes, metadata.Brokers.NodeIDs())
	}

	completed := now.After(state.End)
	var checkError *action_kit_api.ActionKitError

	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}

	if len(state.ExpectedChanges) > 0 {
		if state.StateCheckMode == stateCheckModeAllTheTime {
			for _, c := range keys {
				if !slices.Contains(state.ExpectedChanges, c) {
					checkError = extutil.Ptr(action_kit_api.ActionKitError{
						Title: fmt.Sprintf("Brokers got an unexpected change '%s' whereas '%s' is expected. Change(s) : %v",
							c,
							state.ExpectedChanges,
							changes[c]),
						Status: extutil.Ptr(action_kit_api.Failed),
					})
				}
			}
		} else if state.StateCheckMode == stateCheckModeAtLeastOnce {
			for _, c := range keys {
				if slices.Contains(state.ExpectedChanges, c) {
					state.StateCheckSuccess = true
				}
			}

			if completed && !state.StateCheckSuccess {
				checkError = extutil.Ptr(action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Brokers didn't get the expected changes '%s' at least once.",
						state.ExpectedChanges),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		}
	}

	metrics := []action_kit_api.Metric{
		*toBrokerChangeMetric(state.ExpectedChanges, keys, changes, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   extutil.Ptr(metrics),
	}, nil
}

func toBrokerChangeMetric(expectedChanges []string, changesNames []string, changes map[string][]int32, now time.Time) *action_kit_api.Metric {
	var tooltip string
	var state string

	if len(changes) > 0 {
		recap := "BROKER ACTIVITY"
		for k, v := range changes {
			recap += "\n" + k + ":\n"
			for _, nodeID := range v {
				recap += fmt.Sprint(nodeID) + "\n"
			}

		}

		tooltip = recap

		sort.Strings(expectedChanges)
		sort.Strings(changesNames)

		for _, change := range changesNames {
			if slices.Contains(expectedChanges, change) {
				state = "success"
			} else {
				state = "danger"
			}
		}
	} else {
		tooltip = "No changes"
		state = "info"
	}

	return extutil.Ptr(action_kit_api.Metric{
		Name: extutil.Ptr("kafka_consumer_group_state"),
		Metric: map[string]string{
			"metric.id": MetricID,
			"url":       "",
			"state":     state,
			"tooltip":   tooltip,
		},
		Timestamp: now,
		Value:     0,
	})
}

func areSlicesEqualUnordered(slice1, slice2 []int32) bool {
	if len(slice1) != len(slice2) {
		return false
	}
	sorted1 := slices.Clone(slice1) // Create a copy to avoid modifying the original slices
	sorted2 := slices.Clone(slice2)
	sort.Slice(sorted1, func(i, j int) bool { return sorted1[i] < sorted1[j] })
	sort.Slice(sorted2, func(i, j int) bool { return sorted2[i] < sorted2[j] })
	return slices.Equal(sorted1, sorted2)
}

func findMissingElements(slice1, slice2 []int32) []int32 {
	// Create a map to store elements in slice2
	elementMap := make(map[int32]bool)
	for _, v := range slice2 {
		elementMap[v] = true
	}

	// Find elements in slice1 that are not in slice2
	var missing []int32
	for _, v := range slice1 {
		if !elementMap[v] {
			missing = append(missing, v)
		}
	}

	return missing
}

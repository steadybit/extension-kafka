// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/steadybit/extension-kafka/config"

	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"

	"slices"
	"sort"
	"time"
)

type CheckBrokersAction struct{}

type CheckBrokersState struct {
	PreviousController int32
	BrokerNodes        []int32
	End                time.Time
	ExpectedChanges    []string
	StateCheckMode     string
	StateCheckSuccess  bool
	BrokerHosts        []string
	ClusterName        string // Cluster name for multi-cluster support
}

const (
	BrokerControllerChanged = "kafka controller changed"
	BrokerDowntime          = "kafka broker with downtime"
)

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[CheckBrokersState]           = (*CheckBrokersAction)(nil)
	_ action_kit_sdk.ActionWithStatus[CheckBrokersState] = (*CheckBrokersAction)(nil)
)

func NewBrokersCheckAction() action_kit_sdk.Action[CheckBrokersState] {
	return &CheckBrokersAction{}
}

func (m *CheckBrokersAction) NewEmptyState() CheckBrokersState {
	return CheckBrokersState{}
}

func (m *CheckBrokersAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check", kafkaBrokerTargetId),
		Label:       "Check Brokers",
		Description: "Monitor broker-level changes such as controller elections and broker downtime during an experiment. For topic partition changes, use Check Partitions instead. For consumer group state, use Check Consumer State.",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        new(kafkaIcon),
		TargetSelection: new(action_kit_api.TargetSelection{
			TargetType:          kafkaBrokerTargetId,
			QuantityRestriction: extutil.Ptr(action_kit_api.QuantityRestrictionAll),
			SelectionTemplates: new([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by cluster name",
					Description: new("Find brokers by cluster name"),
					Query:       "kafka.cluster.name=\"\"",
				},
			}),
		}),
		Technology:  new("Kafka"),
		Category:    new("Kafka"),
		Kind:        action_kit_api.Check,
		TimeControl: action_kit_api.TimeControlInternal,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:         "duration",
				Label:        "Duration",
				Description:  new("How long the check runs. Broker state is polled continuously for this duration."),
				Type:         action_kit_api.ActionParameterTypeDuration,
				DefaultValue: new("30s"),
				Required:     new(true),
			},
			{
				Name:        "expectedChanges",
				Label:       "Expected Changes",
				Description: new("Which broker-level changes to watch for. If left empty, any broker change triggers the check."),
				Type:        action_kit_api.ActionParameterTypeStringArray,
				Options: new([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "New Controller Elected",
						Value: BrokerControllerChanged,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Broker downtime",
						Value: BrokerDowntime,
					},
				}),
				Required: new(false),
			},
			{
				Name:         "changeCheckMode",
				Label:        "Change Check Mode",
				Description:  new("How the expected changes are evaluated. 'All the time' means every poll must detect the change. 'At least once' means the change must be observed at least once during the duration."),
				Type:         action_kit_api.ActionParameterTypeString,
				DefaultValue: new(stateCheckModeAllTheTime),
				Options: new([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "All the time",
						Value: stateCheckModeAllTheTime,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "At least once",
						Value: stateCheckModeAtLeastOnce,
					},
				}),
				Required: new(true),
			},
		},
		Widgets: new([]action_kit_api.Widget{
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
				Url: new(action_kit_api.StateOverTimeWidgetUrlConfig{
					From: new("url"),
				}),
				Value: new(action_kit_api.StateOverTimeWidgetValueConfig{
					Hide: new(true),
				}),
			},
		}),
		Status: new(action_kit_api.MutatingEndpointReferenceWithCallInterval{
			CallInterval: new("2s"),
		}),
	}
}

func (m *CheckBrokersAction) Prepare(ctx context.Context, state *CheckBrokersState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
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

	// Get cluster name from target
	clusterName := extutil.MustHaveValue(request.Target.Attributes, "kafka.cluster.name")[0]
	clusterConfig, err := config.GetClusterConfig(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	state.ClusterName = clusterName
	state.BrokerHosts = strings.Split(clusterConfig.SeedBrokers, ",")

	client, err := createNewAdminClientWithConfig(state.BrokerHosts, clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, new(extension_kit.ToError(fmt.Sprintf("Failed to retrieve brokers from Kafka. Full response: %v", err), err))
	}

	state.End = end
	state.ExpectedChanges = expectedState
	state.StateCheckMode = stateCheckMode
	state.StateCheckSuccess = false
	state.PreviousController = metadata.Controller
	state.BrokerNodes = metadata.Brokers.NodeIDs()

	return nil, nil
}

func (m *CheckBrokersAction) Start(_ context.Context, _ *CheckBrokersState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *CheckBrokersAction) Status(ctx context.Context, state *CheckBrokersState) (*action_kit_api.StatusResult, error) {
	return BrokerCheckStatus(ctx, state)
}

func BrokerCheckStatus(ctx context.Context, state *CheckBrokersState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	clusterConfig, err := config.GetClusterConfig(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	client, err := createNewAdminClientWithConfig(state.BrokerHosts, clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, new(extension_kit.ToError(fmt.Sprintf("Failed to retrieve brokers from Kafka. Full response: %v", err), err))
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
					checkError = new(action_kit_api.ActionKitError{
						Title: fmt.Sprintf("Brokers got an unexpected change '%s' whereas '%s' is expected.",
							c,
							state.ExpectedChanges),
						Status: extutil.Ptr(action_kit_api.Failed),
					})
				}
			}
			if len(changes) == 0 {
				checkError = new(action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Brokers got an unexpected change '%v' whereas '%s' is expected.",
						"No changes",
						state.ExpectedChanges),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		} else if state.StateCheckMode == stateCheckModeAtLeastOnce {
			for _, c := range keys {
				if slices.Contains(state.ExpectedChanges, c) {
					state.StateCheckSuccess = true
				}
			}

			if completed && !state.StateCheckSuccess {
				checkError = new(action_kit_api.ActionKitError{
					Title: fmt.Sprintf("Brokers didn't get the expected changes '%s' at least once.",
						state.ExpectedChanges),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		}
	} else {
		if len(changes) > 0 {
			checkError = new(action_kit_api.ActionKitError{
				Title: fmt.Sprintf("Brokers got an unexpected change '%v' whereas '%s' is expected.",
					keys,
					"No changes"),
				Status: extutil.Ptr(action_kit_api.Failed),
			})
		}
	}

	metrics := []action_kit_api.Metric{
		*toBrokerChangeMetric(state.ClusterName, state.ExpectedChanges, keys, changes, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   new(metrics),
	}, nil
}

func toBrokerChangeMetric(clusterName string, expectedChanges []string, changesNames []string, changes map[string][]int32, now time.Time) *action_kit_api.Metric {
	var tooltip string
	var state string

	if len(changes) > 0 {
		var recap strings.Builder
		recap.WriteString("BROKER ACTIVITY\n")
		for k := range changes {
			recap.WriteString(fmt.Sprintf("%s: %s\n", k, joinInt32s(changes[k])))
		}
		tooltip = recap.String()

		sort.Strings(expectedChanges)
		sort.Strings(changesNames)

		state = "warn"
		for _, change := range changesNames {
			if slices.Contains(expectedChanges, change) {
				state = "success"
			}
		}
		if len(expectedChanges) == 0 {
			state = "danger"
		}
	} else {
		tooltip = "No changes"
		state = "info"
	}

	var metricId string
	if len(expectedChanges) > 0 {
		metricId = fmt.Sprintf("%s - Expected: %s", clusterName, strings.Join(expectedChanges, ","))
	} else {
		metricId = fmt.Sprintf("%s - Expected: No changes", clusterName)
	}

	return new(action_kit_api.Metric{
		Name: new("kafka_broker_state"),
		Metric: map[string]string{
			"metric.id": metricId,
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
	slices.Sort(sorted1)
	slices.Sort(sorted2)
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

func joinInt32s(nums []int32) string {
	strs := make([]string, len(nums))
	for i, n := range nums {
		strs[i] = strconv.FormatInt(int64(n), 10)
	}
	return strings.Join(strs, ",")
}

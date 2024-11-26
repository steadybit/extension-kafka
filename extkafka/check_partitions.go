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
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"slices"
	"sort"
	"strings"
	"time"
)

type PartitionsCheckAction struct{}

type PartitionsCheckState struct {
	TopicName               string
	PreviousReplicas        map[int32][]int32
	PreviousInSyncReplicas  map[int32][]int32
	PreviousOfflineReplicas map[int32][]int32
	PreviousLeader          map[int32]int32
	End                     time.Time
	ExpectedChanges         []string
	StateCheckMode          string
	StateCheckSuccess       bool
}

const (
	LeaderChanged          = "leader changed"
	ReplicasChanged        = "replicas changed"
	OfflineReplicasChanged = "offline replicas changed"
	InSyncReplicasChanged  = "in sync replicas changed"
)

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[PartitionsCheckState]           = (*PartitionsCheckAction)(nil)
	_ action_kit_sdk.ActionWithStatus[PartitionsCheckState] = (*PartitionsCheckAction)(nil)
)

func NewPartitionsCheckAction() action_kit_sdk.Action[PartitionsCheckState] {
	return &PartitionsCheckAction{}
}

func (m *PartitionsCheckAction) NewEmptyState() PartitionsCheckState {
	return PartitionsCheckState{}
}

func (m *PartitionsCheckAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check-partitions", kafkaTopicTargetId),
		Label:       "Check Partitions",
		Description: "Check for topic partitions changes like leader, in-sync-replicas, replicas and offline replicas",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType:          kafkaTopicTargetId,
			QuantityRestriction: extutil.Ptr(action_kit_api.All),
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "default",
					Description: extutil.Ptr("Find topic group by name"),
					Query:       "kafka.topic.name=\"\"",
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
				Name:        "expectedChanges",
				Label:       "Expected Changes",
				Description: extutil.Ptr(""),
				Type:        action_kit_api.StringArray,
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "New Leader Elected",
						Value: LeaderChanged,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Replicas Changed",
						Value: ReplicasChanged,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Offline Replicas Changed",
						Value: OfflineReplicasChanged,
					},
					action_kit_api.ExplicitParameterOption{
						Label: "In-Sync Replicas Changed",
						Value: InSyncReplicasChanged,
					},
				}),
				Required: extutil.Ptr(false),
			},
			{
				Name:         "changeCheckMode",
				Label:        "Change Check Mode",
				Description:  extutil.Ptr("How do we check the change of the topic?"),
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
				Title: "Kafka Topic Changes",
				Identity: action_kit_api.StateOverTimeWidgetIdentityConfig{
					From: "kafka.topic.name",
				},
				Label: action_kit_api.StateOverTimeWidgetLabelConfig{
					From: "kafka.topic.name",
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

func (m *PartitionsCheckAction) Prepare(_ context.Context, state *PartitionsCheckState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	if len(request.Target.Attributes["kafka.topic.name"]) == 0 {
		return nil, fmt.Errorf("the target is missing the kafka.topic.name attribute")
	}
	state.TopicName = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.name")[0]

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

	state.End = end
	state.ExpectedChanges = expectedState
	state.StateCheckMode = stateCheckMode
	state.PreviousInSyncReplicas = make(map[int32][]int32)
	state.PreviousReplicas = make(map[int32][]int32)
	state.PreviousOfflineReplicas = make(map[int32][]int32)
	state.PreviousLeader = make(map[int32]int32)

	return nil, nil
}

func (m *PartitionsCheckAction) Start(_ context.Context, _ *PartitionsCheckState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *PartitionsCheckAction) Status(ctx context.Context, state *PartitionsCheckState) (*action_kit_api.StatusResult, error) {
	return TopicCheckStatus(ctx, state)
}

func TopicCheckStatus(ctx context.Context, state *PartitionsCheckState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	client, err := createNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	topics, err := client.ListTopics(ctx, state.TopicName)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve topics from Kafka for name %s. Full response: %v", state.TopicName, err), err))
	}

	var topicDetail kadm.TopicDetail
	if len(topics.Sorted()) == 0 {
		log.Err(err).Msgf("No topic with that name %s.", state.TopicName)
	} else if len(topics.Sorted()) > 1 {
		log.Err(err).Msgf("More than 1 topic with that name %s.", state.TopicName)
	} else {
		topicDetail = topics.Sorted()[0]
	}

	completed := now.After(state.End)
	var checkError *action_kit_api.ActionKitError

	// Check for changes
	changes := make(map[string][]string)
	for _, p := range topicDetail.Partitions.Sorted() {
		if previousReplicas, ok := state.PreviousReplicas[p.Partition]; ok {
			if !slices.Equal(previousReplicas, p.Replicas) {
				changes[ReplicasChanged] = append(changes[ReplicasChanged], fmt.Sprintf("previous: %v,actual: %v", previousReplicas, p.Replicas))
				state.PreviousReplicas[p.Partition] = p.Replicas
			}
		} else {
			state.PreviousReplicas[p.Partition] = p.Replicas
		}

		if previousISR, ok := state.PreviousInSyncReplicas[p.Partition]; ok {
			if !slices.Equal(previousISR, p.ISR) {
				changes[InSyncReplicasChanged] = append(changes[InSyncReplicasChanged], fmt.Sprintf("previous: %v,actual: %v", previousISR, p.ISR))
				state.PreviousInSyncReplicas[p.Partition] = p.ISR
			}
		} else {
			state.PreviousInSyncReplicas[p.Partition] = p.ISR
		}

		if previousOffline, ok := state.PreviousOfflineReplicas[p.Partition]; ok {
			if !slices.Equal(previousOffline, p.OfflineReplicas) {
				changes[OfflineReplicasChanged] = append(changes[OfflineReplicasChanged], fmt.Sprintf("previous: %v,actual: %v", previousOffline, p.OfflineReplicas))
				state.PreviousOfflineReplicas[p.Partition] = p.OfflineReplicas
			}
		} else {
			state.PreviousOfflineReplicas[p.Partition] = p.OfflineReplicas
		}

		if previousLeader, ok := state.PreviousLeader[p.Partition]; ok {
			if previousLeader != p.Leader {
				changes[LeaderChanged] = append(changes[LeaderChanged], fmt.Sprintf("previous: %v,actual: %v", previousLeader, p.Leader))
				state.PreviousLeader[p.Partition] = p.Leader
			}
		} else {
			state.PreviousLeader[p.Partition] = p.Leader
		}
	}

	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}

	if len(state.ExpectedChanges) > 0 {
		if state.StateCheckMode == stateCheckModeAllTheTime {
			for _, c := range keys {
				if !slices.Contains(state.ExpectedChanges, c) {
					checkError = extutil.Ptr(action_kit_api.ActionKitError{
						Title: fmt.Sprintf("Topic '%s' has an unexpected change '%s' whereas '%s' is expected. Change(s) : %v",
							state.TopicName,
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
					Title: fmt.Sprintf("Topic '%s' didn't have the expected changes '%s' at least once.",
						state.TopicName,
						state.ExpectedChanges),
					Status: extutil.Ptr(action_kit_api.Failed),
				})
			}
		}
	}

	metrics := []action_kit_api.Metric{
		*toTopicChangeMetric(state.TopicName, state.ExpectedChanges, keys, changes, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   extutil.Ptr(metrics),
	}, nil
}

func toTopicChangeMetric(topicName string, expectedChanges []string, changesNames []string, changes map[string][]string, now time.Time) *action_kit_api.Metric {
	var tooltip string
	var state string

	if len(changes) > 0 {
		recap := "PARTITION ACTIVITY"
		for k, v := range changes {
			recap += "\n" + k + ":\n"
			recap += strings.Join(v, "\n")
		}

		tooltip = recap

		sort.Strings(expectedChanges)
		sort.Strings(changesNames)

		if slices.Equal(expectedChanges, changesNames) {
			state = "success"
		} else {
			state = "danger"
		}
	} else {
		tooltip = "No changes"
		state = "info"
	}

	return extutil.Ptr(action_kit_api.Metric{
		Name: extutil.Ptr("kafka_consumer_group_state"),
		Metric: map[string]string{
			"kafka.topic.name": topicName,
			"url":              "",
			"state":            state,
			"tooltip":          tooltip,
		},
		Timestamp: now,
		Value:     0,
	})
}

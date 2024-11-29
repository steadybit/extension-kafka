// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"strconv"
	"strings"
)

type DeleteRecordsAttack struct{}

type DeleteRecordsState struct {
	TopicName   string
	Partitions  []string
	Offset      int64
	BrokerHosts []string
}

var _ action_kit_sdk.Action[DeleteRecordsState] = (*DeleteRecordsAttack)(nil)

func NewDeleteRecordsAttack() action_kit_sdk.Action[DeleteRecordsState] {
	return &DeleteRecordsAttack{}
}

func (k *DeleteRecordsAttack) NewEmptyState() DeleteRecordsState {
	return DeleteRecordsState{}
}

func (k *DeleteRecordsAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.delete-records", kafkaTopicTargetId),
		Label:       "Trigger Delete Records",
		Description: "Trigger delete records to move the offset relative to the last known offset for each selected partition",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaTopicTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by topic name",
					Description: extutil.Ptr("Find topic by name"),
					Query:       "kafka.topic.name=\"\"",
				},
			}),
		}),
		Technology:  extutil.Ptr("Kafka"),
		Category:    extutil.Ptr("Kafka"),
		TimeControl: action_kit_api.TimeControlInstantaneous,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:        "partitions",
				Label:       "Partition to issue delete records requests",
				Description: extutil.Ptr("One or more partitions to delete the records"),
				Type:        action_kit_api.StringArray,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.topic.partitions",
					},
				}),
			},
			{
				Label:        "X from newest Offset",
				Description:  extutil.Ptr("To move the offset in the past, 0 means to the last known offset (skipping every records from where the consumer was)."),
				Name:         "offset",
				Type:         action_kit_api.Integer,
				DefaultValue: extutil.Ptr("0"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *DeleteRecordsAttack) Prepare(_ context.Context, state *DeleteRecordsState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.TopicName = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.name")[0]
	state.Partitions = extutil.ToStringArray(request.Config["partitions"])
	state.Offset = extutil.ToInt64(request.Config["offset"])
	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")

	return nil, nil
}

func (k *DeleteRecordsAttack) Start(ctx context.Context, state *DeleteRecordsState) (*action_kit_api.StartResult, error) {
	var errs []error

	adminClient, err := createNewAdminClient(state.BrokerHosts)
	if err != nil {
		return nil, err
	}
	defer adminClient.Close()

	// Get Current offset
	endOffsets, err := adminClient.ListEndOffsets(ctx, state.TopicName)
	if err != nil {
		return nil, err
	}
	if endOffsets.Error() != nil {
		return nil, endOffsets.Error()
	}

	var logMessages []string
	// For each partition, fetch the offset
	for _, partition := range state.Partitions {
		var partitionInt int64
		partitionInt, err = strconv.ParseInt(partition, 10, 64)
		if err != nil {
			return nil, extension_kit.ToError(fmt.Sprintf("Failed to convert partition %s to int32", partition), err)
		}

		endOffset, found := endOffsets.Lookup(state.TopicName, int32(partitionInt))
		if !found {
			return nil, extension_kit.ToError(fmt.Sprintf("Failed to find offset for topic %s and partition %s", state.TopicName, partition), nil)
		}

		newOffsets := kadm.Offsets{}
		newOffset := endOffset.Offset - state.Offset
		newOffsets.Add(kadm.Offset{Topic: endOffset.Topic, Partition: endOffset.Partition, LeaderEpoch: endOffset.LeaderEpoch, At: newOffset})

		var offsetResponses kadm.DeleteRecordsResponses
		offsetResponses, err = adminClient.DeleteRecords(ctx, newOffsets)
		if err != nil {
			return nil, err
		}

		for _, newOffsetResponse := range offsetResponses.Sorted() {
			if newOffsetResponse.Err != nil {
				errs = append(errs, newOffsetResponse.Err)
			}
		}
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		logMessages = append(logMessages, fmt.Sprintf("Trigger delete records for topic %s for partition %v, moving offset at %d", state.TopicName, partition, newOffset))
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: strings.Join(logMessages, "\n"),
		}},
	}, nil

}

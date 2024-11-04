// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"strconv"
)

type DeleteRecordsAttack struct{}

type DeleteRecordsState struct {
	BrokerID   int32
	TopicName  string
	Partitions []string
	Offset     string
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
		Description: "Trigger a delete records request for the given offsets",
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
		TimeControl: action_kit_api.TimeControlInstantaneous,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:        "partitions",
				Label:       "Partition to elect new leader",
				Description: extutil.Ptr("One or more partitions to trigger a new leader election"),
				Type:        action_kit_api.StringArray,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.topic.partitions",
					},
				}),
				Order: extutil.Ptr(2),
			},
			{
				Label:        "Offset",
				Description:  extutil.Ptr("To delete records, Kafka sets the LogStartOffset for partitions to the requested offset. All segments whose max partition is before the requested offset are deleted, and any records within the segment before the requested offset can no longer be read."),
				Name:         "offset",
				Type:         action_kit_api.Integer,
				DefaultValue: extutil.Ptr("10"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *DeleteRecordsAttack) Prepare(_ context.Context, state *DeleteRecordsState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	state.Partitions = extutil.ToStringArray(request.Config["partitions"])

	return nil, nil
}

func (k *DeleteRecordsAttack) Start(ctx context.Context, state *DeleteRecordsState) (*action_kit_api.StartResult, error) {
	adminClient, err := CreateNewAdminClient()
	if err != nil {
		return nil, err
	}

	offsets := kadm.Offsets{}
	for _, partition := range state.Partitions {
		var partitionInt int64
		var offset int64
		partitionInt, err = strconv.ParseInt(partition, 10, 32)
		if err != nil {
			return nil, err
		}
		offset, err = strconv.ParseInt(state.Offset, 10, 32)
		if err != nil {
			return nil, err
		}
		offsets.Add(kadm.Offset{Topic: state.TopicName, Partition: int32(partitionInt), At: offset})
	}
	var recordsResponse kadm.DeleteRecordsResponses
	recordsResponse, err = adminClient.DeleteRecords(ctx, offsets)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, response := range recordsResponse.Sorted() {
		if response.Err != nil {
			errs = append(errs, response.Err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Trigger delete records for topic:  %s for partitions %v at offset %s", state.TopicName, state.Partitions, state.Offset),
		}},
	}, nil

}

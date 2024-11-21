// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"strconv"
)

type kafkaBrokerElectNewLeaderAttack struct {
}

var _ action_kit_sdk.Action[KafkaBrokerAttackState] = (*kafkaBrokerElectNewLeaderAttack)(nil)

func NewKafkaBrokerElectNewLeaderAttack() action_kit_sdk.Action[KafkaBrokerAttackState] {
	return kafkaBrokerElectNewLeaderAttack{}
}

func (f kafkaBrokerElectNewLeaderAttack) NewEmptyState() KafkaBrokerAttackState {
	return KafkaBrokerAttackState{}
}

func (f kafkaBrokerElectNewLeaderAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.elect-new-leader", kafkaTopicTargetId),
		Label:       "Elect New Partition Leader",
		Description: "Elect a new leader for a given topic and partition(s), the leader must be unavailable for the election to succeed.",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaTopicTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by topic  id",
					Description: extutil.Ptr("Find topic by id"),
					Query:       "kafka.topic.id=\"\"",
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
		},
	}
}

func (f kafkaBrokerElectNewLeaderAttack) Prepare(_ context.Context, state *KafkaBrokerAttackState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.Topic = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.name")[0]
	state.Partitions = extutil.ToStringArray(request.Config["partitions"])
	return nil, nil
}

func (f kafkaBrokerElectNewLeaderAttack) Start(ctx context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StartResult, error) {
	messages := make([]action_kit_api.Message, 0)
	client, err := createNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Parse partitions
	partitions, err := sliceAtoi(state.Partitions)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partitions: %s", err.Error())
	}

	// Create a slice of TopicPartition
	topicSet := make(kadm.TopicsSet)

	topicSet.Add(state.Topic, partitions...)

	results, err := client.ElectLeaders(ctx, kadm.ElectPreferredReplica, topicSet)
	if err != nil {
		return nil, fmt.Errorf("failed to elect new leader for topic %s and partitions %s: %s", state.Topic, state.Partitions, err)
	}

	for t, parts := range results {
		for partition, result := range parts {
			if result.Err != nil {
				messages = append(messages, action_kit_api.Message{
					Level:   extutil.Ptr(action_kit_api.Error),
					Message: fmt.Sprintf("Error while electing leader for topic '%s', partition %d, error is: %s", t, partition, result.Err.Error()),
				})
			} else {
				messages = append(messages, action_kit_api.Message{
					Level:   extutil.Ptr(action_kit_api.Info),
					Message: fmt.Sprintf("Successfully elected leader for topic '%s', partition %d", t, partition),
				})
			}
		}
	}

	return &action_kit_api.StartResult{
		Messages: &messages,
	}, nil

}

func sliceAtoi(sa []string) ([]int32, error) {
	si := make([]int32, 0, len(sa))
	for _, a := range sa {
		i, err := strconv.Atoi(a)
		if err != nil {
			return si, err
		}
		si = append(si, int32(i))
	}
	return si, nil
}

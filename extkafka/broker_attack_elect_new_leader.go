// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"strconv"
	"time"
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
		Label:       "Trigger Election for new leader",
		Description: "Triggers election for a new leader for a given topic and partition(s)",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaBrokerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by topic  id",
					Description: extutil.Ptr("Find topic by id"),
					Query:       "kafka.topic.id=\"\"",
				},
			}),
		}),
		Category:    extutil.Ptr("resource"),
		TimeControl: action_kit_api.TimeControlInstantaneous,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:        "Topic",
				Label:       "Topic of the partition(s) to elect new leader",
				Description: extutil.Ptr("Topic where the partition must have a new leader"),
				Type:        action_kit_api.String,
				Required:    extutil.Ptr(true),
				Order:       extutil.Ptr(1),
			},
			{
				Name:        "Partition",
				Label:       "Partition to elect new leader",
				Description: extutil.Ptr("One or more partitions to trigger a new leader election"),
				Type:        action_kit_api.StringArray,
				Required:    extutil.Ptr(false),
				Order:       extutil.Ptr(2),
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
	opts := []kgo.Opt{
		kgo.SeedBrokers(config.Config.SeedBrokers),
		kgo.DefaultProduceTopic("steadybit"),
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

	// Parse partitions
	partitions, err := sliceAtoi(state.Partitions)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partitions: %s", err.Error())
	}

	// Create a slice of TopicPartition
	topicSet := make(kadm.TopicsSet)

	topicSet.Add(state.Topic, partitions...)

	results, err := adminClient.ElectLeaders(ctx, kadm.ElectPreferredReplica, topicSet)
	if err != nil {
		return nil, fmt.Errorf("failed to elect new leader for topic %s and partitions %d: %s", state.Topic, state.Partitions, err)
	}

	for topic, partitions := range results {
		for partition, result := range partitions {
			if result.Err != nil {
				return nil, fmt.Errorf("Error electing leader for topic '%s', partition %d: %v\n",
					topic, partition, result.Err)
			} else {
				fmt.Printf("Successfully elected leader for topic '%s', partition %d\n",
					topic, partition)
			}
		}
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Elect new leader for topic %s and partitions %d triggered", state.Topic, state.Partitions),
		}},
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

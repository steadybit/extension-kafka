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
		Id:          fmt.Sprintf("%s.failover", kafkaBrokerTargetId),
		Label:       "Trigger Failover",
		Description: "Triggers nodegroup failover by promoting a replica node to primary",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaBrokerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by elasticache nodegroup id",
					Description: extutil.Ptr("Find node groups by replication group id and node group id"),
					Query:       "aws.elasticache.replication-group.id=\"\" and aws.elasticache.replication-group.node-group.id=\"\"",
				},
			}),
		}),
		Category:    extutil.Ptr("resource"),
		TimeControl: action_kit_api.TimeControlInstantaneous,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:         "Topic",
				Label:        "Topic to elect new leader",
				Description:  extutil.Ptr("List of topics to elect a new leader, if none, will impact all topics."),
				Type:         action_kit_api.String,
				DefaultValue: extutil.Ptr("1"),
				Required:     extutil.Ptr(false),
				Order:        extutil.Ptr(1),
			},
			{
				Name:         "Partition",
				Label:        "Partition to elect new leader",
				Description:  extutil.Ptr("List of topics to elect a new leader, if none, will impact all topics."),
				Type:         action_kit_api.String,
				DefaultValue: extutil.Ptr("1"),
				Required:     extutil.Ptr(false),
				Order:        extutil.Ptr(1),
			},
		},
	}
}

func (f kafkaBrokerElectNewLeaderAttack) Prepare(_ context.Context, state *KafkaBrokerAttackState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.Topic = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.id")[0]
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

	// Create a slice of TopicPartition
	topicSet := make(kadm.TopicsSet)
	topicSet.Add(state.Topic, state.Partitions...)

	_, err = adminClient.ElectLeaders(ctx, kadm.ElectPreferredReplica, topicSet)
	if err != nil {
		return nil, fmt.Errorf("failed to elect new leader for topic %s and partitions %d: %s", state.Topic, state.Partitions, err)
	}
	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Elect new leader for topic %s and partitions %d triggered", state.Topic, state.Partitions),
		}},
	}, nil

}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
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
		Description: "Elect a new leader for a given topic and partition, only by trying to elect a new preferred replica. The current leader of the partition will be placed at the end of replicas through a reassignment",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaTopicTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "topic id",
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
				Label:       "Partition to elect a new leader (preferred replica)",
				Description: extutil.Ptr("The partition to elect a new leader"),
				Type:        action_kit_api.ActionParameterTypeString,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.topic.partitions",
					},
				}),
			},
		},
	}
}

func (f kafkaBrokerElectNewLeaderAttack) Prepare(_ context.Context, state *KafkaBrokerAttackState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.Topic = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.name")[0]
	state.Partition = extutil.ToInt32(request.Config["partitions"])

	// Get cluster name from target
	clusterName := extutil.MustHaveValue(request.Target.Attributes, "kafka.cluster.name")[0]
	clusterConfig, err := config.GetClusterConfig(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	state.ClusterName = clusterName
	state.BrokerHosts = strings.Split(clusterConfig.SeedBrokers, ",")

	return nil, nil
}

func (f kafkaBrokerElectNewLeaderAttack) Start(ctx context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StartResult, error) {
	messages := make([]action_kit_api.Message, 0)

	clusterConfig, err := config.GetClusterConfig(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	client, err := createNewAdminClientWithConfig(state.BrokerHosts, clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Find the corresponding leader info
	topics, err := client.ListTopics(ctx, state.Topic)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve topics from Kafka for name %s. Full response: %v", state.Topic, err), err))
	}

	var topicDetail kadm.TopicDetail
	if len(topics.Sorted()) == 0 {
		log.Err(err).Msgf("No topic with that name %s.", state.Topic)
	} else if len(topics.Sorted()) > 1 {
		log.Err(err).Msgf("More than 1 topic with that name %s.", state.Topic)
	} else {
		topicDetail = topics.Sorted()[0]
	}
	partition := topicDetail.Partitions[state.Partition]

	assignment := kadm.AlterPartitionAssignmentsReq{}
	assignment.Assign(state.Topic, state.Partition, relegateLeader(partition.Replicas, partition.ISR, partition.Leader))
	// Reassign the leader to the end of replicas preferences, to give a chance to another broker to become leader
	_, err = client.AlterPartitionAssignments(ctx, assignment)
	if err != nil {
		return nil, err
	}

	topicSet := make(kadm.TopicsSet)
	topicSet.Add(state.Topic, []int32{state.Partition}...)

	results, err := client.ElectLeaders(ctx, kadm.ElectPreferredReplica, topicSet)
	if err != nil {
		return nil, fmt.Errorf("failed to elect new leader for topic %s and partition %d: %s", state.Topic, state.Partition, err)
	}
	var errorElectLeader action_kit_api.ActionKitError
	var errs []error
	for t, parts := range results {
		for partition, result := range parts {
			if result.Err != nil {
				messages = append(messages, action_kit_api.Message{
					Level:   extutil.Ptr(action_kit_api.Warn),
					Message: fmt.Sprintf("Error while electing leader for topic '%s', partition %d, error is: %s", t, partition, result.Err.Error()),
				})
				errs = append(errs, fmt.Errorf("error while electing leader for topic '%s', partition %d, error is: %s", t, partition, result.Err.Error()))
			} else {
				messages = append(messages, action_kit_api.Message{
					Level:   extutil.Ptr(action_kit_api.Info),
					Message: fmt.Sprintf("Successfully elected leader for topic '%s', partition %d.", t, partition),
				})
			}
		}
	}
	if len(errs) > 0 {
		errorElectLeader = action_kit_api.ActionKitError{Title: "Election failed for partition(s)", Detail: extutil.Ptr(errors.Join(errs...).Error())}
		return &action_kit_api.StartResult{
			Messages: &messages,
			Error:    &errorElectLeader,
		}, nil
	}

	return &action_kit_api.StartResult{
		Messages: &messages,
	}, nil

}

func relegateLeader(replicas []int32, replicaInSync []int32, leader int32) []int32 {
	var brokers []int32
	// Add first the next in sync replicas
	for _, ns := range replicas {
		if ns != leader && slices.Contains(replicaInSync, ns) {
			brokers = append(brokers, ns)
		}
	}
	// Then add not in-sync replicas
	for _, ns := range replicas {
		if !slices.Contains(replicaInSync, ns) && ns != leader {
			brokers = append(brokers, ns)
		}
	}
	// and add last the leader
	return append(brokers, leader)
}

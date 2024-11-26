// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"regexp"
	"slices"
	"strconv"
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
				Label:       "Partition to elect a new leader (preferred replica)",
				Description: extutil.Ptr("The partition to elect a new leader"),
				Type:        action_kit_api.String,
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
	state.Partitions = extutil.ToString(request.Config["partitions"])
	state.ElectLeadersHow = extutil.ToString(request.Config["election_method"])
	state.PartitionsInSyncReplicas = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.partitions-isr")
	state.PartitionsReplicas = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.partitions-replicas")
	state.PartitionsLeaders = extutil.MustHaveValue(request.Target.Attributes, "kafka.topic.partitions-leaders")
	return nil, nil
}

func (f kafkaBrokerElectNewLeaderAttack) Start(ctx context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StartResult, error) {
	messages := make([]action_kit_api.Message, 0)
	client, err := createNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Find the corresponding leader info
	var leader string
	leader = strings.Split(retrievePartitionInfo(state.Partitions, state.PartitionsLeaders), "=")[1]

	var partitionInt int64
	partitionInt, err = strconv.ParseInt(state.Partitions, 10, 64)
	if err != nil {
		return nil, extension_kit.ToError(fmt.Sprintf("Failed to convert partition %s to int32", state.Partitions), err)
	}

	replicaArray := strings.Fields(extractBrackets(retrievePartitionInfo(state.Partitions, state.PartitionsReplicas)))
	replicaInSyncArray := strings.Fields(extractBrackets(retrievePartitionInfo(state.Partitions, state.PartitionsInSyncReplicas)))

	assignment := kadm.AlterPartitionAssignmentsReq{}
	assignment.Assign(state.Topic, int32(partitionInt), relegateLeaderForPreferredReplicaElection(replicaArray, replicaInSyncArray, leader))
	// Reassign the leader to the end of replicas preferences, to give a chance to another broker to become leader
	_, err = client.AlterPartitionAssignments(ctx, assignment)
	if err != nil {
		return nil, err
	}

	topicSet := make(kadm.TopicsSet)
	topicSet.Add(state.Topic, []int32{int32(partitionInt)}...)

	results, err := client.ElectLeaders(ctx, kadm.ElectPreferredReplica, topicSet)
	if err != nil {
		return nil, fmt.Errorf("failed to elect new leader for topic %s and partitions %s: %s", state.Topic, state.Partitions, err)
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

func retrievePartitionInfo(p string, partitionInfos []string) string {
	for _, partition := range partitionInfos {
		if strings.HasPrefix(partition, p) {
			return partition
		}
	}
	return ""
}

func extractBrackets(p string) string {
	// Regular expression to find content between brackets
	re := regexp.MustCompile(`\[(.*?)\]`)
	matches := re.FindStringSubmatch(p)
	return matches[1]
}

func relegateLeaderForPreferredReplicaElection(replicas []string, replicaInSync []string, leader string) []int32 {
	leaderInt, _ := strconv.ParseInt(leader, 10, 64)

	var numbers []int32
	// Add first the in sync replicas
	for _, ns := range replicas {
		num, err := strconv.Atoi(ns)
		if err != nil {
			fmt.Printf("Error converting '%s' to integer: %v\n", ns, err)
		}
		if ns != leader && slices.Contains(replicaInSync, ns) {
			numbers = append(numbers, int32(num))
		}
	}
	// Then add the leader and not in-sync
	for _, ns := range replicas {
		num, err := strconv.Atoi(ns)
		if err != nil {
			fmt.Printf("Error converting '%s' to integer: %v\n", ns, err)
		}
		if !slices.Contains(replicaInSync, ns) && ns != leader {
			numbers = append(numbers, int32(num))
		}
	}
	return append(numbers, int32(leaderInt))
}

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
)

type KafkaConsumerDenyAccessAttack struct{}

type KafkaDenyUserState struct {
	ConsumerGroup string
	Topic         string
	User          string
}

var _ action_kit_sdk.Action[KafkaDenyUserState] = (*KafkaConsumerDenyAccessAttack)(nil)

func NewKafkaConsumerDenyAccessAttack() action_kit_sdk.Action[KafkaDenyUserState] {
	return &KafkaConsumerDenyAccessAttack{}
}

func (k *KafkaConsumerDenyAccessAttack) NewEmptyState() KafkaDenyUserState {
	return KafkaDenyUserState{}
}

func (k *KafkaConsumerDenyAccessAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.deny-access", kafkaConsumerTargetId),
		Label:       "Deny Access",
		Description: "Deny access for one or many consumer groups to all hosts",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaConsumerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "default",
					Description: extutil.Ptr("Find consumer group by name"),
					Query:       "kafka.consumer-group.name=\"\"",
				},
			}),
		}),
		Technology:  extutil.Ptr("Message Queue"),
		Category:    extutil.Ptr("Kafka"),
		TimeControl: action_kit_api.TimeControlExternal,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Label:        "Duration",
				Description:  extutil.Ptr("The duration of the action. The broker configuration will be reverted at the end of the action."),
				Name:         "duration",
				Type:         action_kit_api.Duration,
				DefaultValue: extutil.Ptr("180s"),
				Required:     extutil.Ptr(true),
			},
			{
				Label:       "User",
				Description: extutil.Ptr("The user affected by the ACL."),
				Name:        "user",
				Type:        action_kit_api.String,
				Required:    extutil.Ptr(true),
			},
			{
				Label:       "Topic to deny access",
				Name:        "topic",
				Description: extutil.Ptr("One topic to deny access to"),
				Type:        action_kit_api.String,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.consumer-group.topics",
					},
				}),
			},
		},
	}
}

func (k *KafkaConsumerDenyAccessAttack) Prepare(_ context.Context, state *KafkaDenyUserState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	if len(request.Target.Attributes["kafka.consumer-group.name"]) == 0 {
		return nil, fmt.Errorf("the target is missing the kafka.consumer-group.name attribute")
	}
	state.ConsumerGroup = request.Target.Attributes["kafka.consumer-group.name"][0]
	state.Topic = extutil.ToString(request.Config["topic"])
	state.User = extutil.ToString(request.Config["user"])

	return nil, nil
}

func (k *KafkaConsumerDenyAccessAttack) Start(ctx context.Context, state *KafkaDenyUserState) (*action_kit_api.StartResult, error) {
	client, err := createNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	acl := kadm.NewACLs().
		ResourcePatternType(kadm.ACLPatternLiteral).
		Topics(state.Topic).
		Groups(state.ConsumerGroup).
		Operations(kadm.OpRead, kadm.OpWrite, kadm.OpDescribe).
		Deny("User:" + state.User).DenyHosts()

	results, err := client.CreateACLs(ctx, acl)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, result := range results {
		if result.Err != nil {
			detailedError := errors.New(result.Err.Error() + result.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Deny access for consumer group  %s for every broker hosts", state.ConsumerGroup),
		}},
	}, nil

}

func (k *KafkaConsumerDenyAccessAttack) Stop(ctx context.Context, state *KafkaDenyUserState) (*action_kit_api.StopResult, error) {
	client, err := createNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	acl := kadm.NewACLs().
		ResourcePatternType(kadm.ACLPatternLiteral).
		Topics(state.Topic).
		Groups(state.ConsumerGroup).
		Operations(kadm.OpRead, kadm.OpWrite, kadm.OpDescribe).
		Deny("User:" + state.User).DenyHosts()

	results, err := client.DeleteACLs(ctx, acl)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, result := range results {
		if result.Err != nil {
			detailedError := errors.New(result.Err.Error() + result.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return nil, nil
}

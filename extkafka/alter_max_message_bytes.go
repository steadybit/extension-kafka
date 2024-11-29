// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"strings"
)

type AlterMessageMaxBytesAttack struct{}

const (
	MessageMaxBytes = "message.max.bytes"
)

var _ action_kit_sdk.Action[AlterState] = (*AlterMessageMaxBytesAttack)(nil)

func NewAlterMaxMessageBytesAttack() action_kit_sdk.Action[AlterState] {
	return &AlterMessageMaxBytesAttack{}
}

func (k *AlterMessageMaxBytesAttack) NewEmptyState() AlterState {
	return AlterState{}
}

func (k *AlterMessageMaxBytesAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.reduce-message-max-bytes", kafkaBrokerTargetId),
		Label:       "Reduce Message Batch Size",
		Description: "Reduce the max bytes allowed per message",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaBrokerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by broker node id",
					Description: extutil.Ptr("Find broker by node id"),
					Query:       "kafka.broker.node-id=\"\"",
				},
			}),
		}),
		Technology:  extutil.Ptr("Kafka"),
		Category:    extutil.Ptr("Kafka"),
		TimeControl: action_kit_api.TimeControlExternal,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			durationAlter,
			{
				Label:        "Max bytes per message",
				Description:  extutil.Ptr("Set a very low max bytes per message to simulate message size rejections."),
				Name:         "max_bytes",
				Type:         action_kit_api.Integer,
				DefaultValue: extutil.Ptr("100"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *AlterMessageMaxBytesAttack) Prepare(_ context.Context, state *AlterState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	state.BrokerConfigValue = fmt.Sprintf("%.0f", request.Config["max_bytes"])
	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")

	return nil, nil
}

func (k *AlterMessageMaxBytesAttack) Start(ctx context.Context, state *AlterState) (*action_kit_api.StartResult, error) {
	var err error
	state.InitialBrokerConfigValue, err = saveConfig(ctx, state.BrokerHosts, MessageMaxBytes, state.BrokerID)
	if err != nil {
		return nil, err
	}

	err = alterConfig(ctx, state.BrokerHosts, MessageMaxBytes, state.BrokerConfigValue, state.BrokerID)
	if err != nil {
		return nil, err
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config "+MessageMaxBytes+" with value %s (initial value was: %s) for broker node-id: %v", state.BrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil

}

func (k *AlterMessageMaxBytesAttack) Stop(ctx context.Context, state *AlterState) (*action_kit_api.StopResult, error) {
	err := alterConfig(ctx, state.BrokerHosts, MessageMaxBytes, state.InitialBrokerConfigValue, state.BrokerID)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

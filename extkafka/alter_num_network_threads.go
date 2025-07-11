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
	"strings"
)

type AlterNumberNetworkThreadsAttack struct{}

const (
	NumberNetworkThreads = "num.network.threads"
)

var _ action_kit_sdk.Action[AlterState] = (*AlterNumberNetworkThreadsAttack)(nil)

func NewAlterNumberNetworkThreadsAttack() action_kit_sdk.Action[AlterState] {
	return &AlterNumberNetworkThreadsAttack{}
}

func (k *AlterNumberNetworkThreadsAttack) NewEmptyState() AlterState {
	return AlterState{}
}

func (k *AlterNumberNetworkThreadsAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.limit-network-threads", kafkaBrokerTargetId),
		Label:       "Limit Network Threads",
		Description: "Limit the number of network threads",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaBrokerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "broker node id",
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
				Label:        "Number of Network Threads",
				Description:  extutil.Ptr("Reduce the num.network.threads to limit the broker’s ability to process network requests."),
				Name:         "network_threads",
				Type:         action_kit_api.ActionParameterTypeInteger,
				DefaultValue: extutil.Ptr("4"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *AlterNumberNetworkThreadsAttack) Prepare(_ context.Context, state *AlterState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	state.BrokerConfigValue = fmt.Sprintf("%.0f", request.Config["network_threads"])
	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")

	return nil, nil
}

func (k *AlterNumberNetworkThreadsAttack) Start(ctx context.Context, state *AlterState) (*action_kit_api.StartResult, error) {
	var err error
	state.InitialBrokerConfigValue, err = saveConfig(ctx, state.BrokerHosts, NumberNetworkThreads, state.BrokerID)
	if err != nil {
		return nil, err
	}

	err = alterConfig(ctx, state.BrokerHosts, NumberNetworkThreads, state.BrokerConfigValue, state.BrokerID)
	if err != nil {
		return nil, err
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config "+NumberNetworkThreads+" with value %s (initial value was: %s) for broker node-id: %v", state.BrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil

}

func (k *AlterNumberNetworkThreadsAttack) Stop(ctx context.Context, state *AlterState) (*action_kit_api.StopResult, error) {
	err := alterConfig(ctx, state.BrokerHosts, NumberNetworkThreads, state.InitialBrokerConfigValue, state.BrokerID)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

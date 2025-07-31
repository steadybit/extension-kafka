// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

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

type AlterNumberIOThreadsAttack struct{}

const (
	NumberIOThreads = "num.io.threads"
)

var _ action_kit_sdk.Action[AlterState] = (*AlterNumberIOThreadsAttack)(nil)

func NewAlterNumberIOThreadsAttack() action_kit_sdk.Action[AlterState] {
	return &AlterNumberIOThreadsAttack{}
}

func (k *AlterNumberIOThreadsAttack) NewEmptyState() AlterState {
	return AlterState{}
}

func (k *AlterNumberIOThreadsAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.limit-io-threads", kafkaBrokerTargetId),
		Label:       "Limit IO Threads",
		Description: "Limit the number of IO threads",
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
				Label:        "Number of IO Threads",
				Description:  extutil.Ptr("Reduce the number of I/O threads to limit the broker’s capacity to perform disk operations, potentially causing increased latency or request timeouts."),
				Name:         "io_threads",
				Type:         action_kit_api.ActionParameterTypeInteger,
				DefaultValue: extutil.Ptr("4"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *AlterNumberIOThreadsAttack) Prepare(ctx context.Context, state *AlterState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	var err error
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	state.BrokerHosts = strings.Split(config.Config.SeedBrokers, ",")
	state.TargetBrokerConfigValue = extutil.ToInt(request.Config["io_threads"])
	state.InitialBrokerConfigValue, err = describeConfigInt(ctx, state.BrokerHosts, NumberIOThreads, state.BrokerID)
	return nil, err
}

func (k *AlterNumberIOThreadsAttack) Start(ctx context.Context, state *AlterState) (*action_kit_api.StartResult, error) {
	if err := adjustThreads(ctx, state.BrokerHosts, NumberIOThreads, state.TargetBrokerConfigValue, state.BrokerID); err != nil {
		return nil, err
	}
	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config %s with value %d (initial value was: %d) for broker node-id: %v", NumberIOThreads, state.TargetBrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil
}

func (k *AlterNumberIOThreadsAttack) Stop(ctx context.Context, state *AlterState) (*action_kit_api.StopResult, error) {
	if err := adjustThreads(ctx, state.BrokerHosts, NumberIOThreads, state.InitialBrokerConfigValue, state.BrokerID); err != nil {
		return nil, err
	}
	return &action_kit_api.StopResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config %s back to initial value %d for broker node-id: %v", NumberIOThreads, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil
}

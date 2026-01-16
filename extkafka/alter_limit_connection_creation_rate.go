// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
)

type AlterLimitConnectionCreateRateAttack struct{}

const (
	LimitConnectionRate = "max.connection.creation.rate"
)

var _ action_kit_sdk.Action[AlterState] = (*AlterLimitConnectionCreateRateAttack)(nil)

func NewAlterLimitConnectionCreateRateAttack() action_kit_sdk.Action[AlterState] {
	return &AlterLimitConnectionCreateRateAttack{}
}

func (k *AlterLimitConnectionCreateRateAttack) NewEmptyState() AlterState {
	return AlterState{}
}

func (k *AlterLimitConnectionCreateRateAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.limit-connection-creation", kafkaBrokerTargetId),
		Label:       "Limit Connection Creation Rate",
		Description: "Limit the Connection Creation Rate",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaBrokerTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "broker node id",
					Description: extutil.Ptr("Find broker by cluster name and id"),
					Query:       "kafka.cluster.name=\"\" AND kafka.broker.node-id=\"\"",
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
				Label:        "Connection creation rate",
				Description:  extutil.Ptr("Limit the connection creation rate to simulate slow acceptance of new connections."),
				Name:         "connection_rate",
				Type:         action_kit_api.ActionParameterTypeInteger,
				DefaultValue: extutil.Ptr("10"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *AlterLimitConnectionCreateRateAttack) Prepare(ctx context.Context, state *AlterState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	var err error
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])

	// Get cluster name from target
	clusterName := extutil.MustHaveValue(request.Target.Attributes, "kafka.cluster.name")[0]
	clusterConfig, err := config.GetClusterConfig(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	state.ClusterName = clusterName
	state.BrokerHosts = strings.Split(clusterConfig.SeedBrokers, ",")
	state.TargetBrokerConfigValue = extutil.ToInt(request.Config["connection_rate"])
	state.InitialBrokerConfigValue, err = describeConfigIntWithConfig(ctx, state.BrokerHosts, LimitConnectionRate, state.BrokerID, clusterConfig)
	return nil, err
}

func (k *AlterLimitConnectionCreateRateAttack) Start(ctx context.Context, state *AlterState) (*action_kit_api.StartResult, error) {
	clusterConfig, err := config.GetClusterConfig(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	if err := alterConfigIntWithConfig(ctx, state.BrokerHosts, LimitConnectionRate, state.TargetBrokerConfigValue, state.BrokerID, clusterConfig); err != nil {
		return nil, err
	}
	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config %s with value %d (initial value was: %d) for broker node-id: %v", LimitConnectionRate, state.TargetBrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil
}

func (k *AlterLimitConnectionCreateRateAttack) Stop(ctx context.Context, state *AlterState) (*action_kit_api.StopResult, error) {
	clusterConfig, err := config.GetClusterConfig(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	err = alterConfigIntWithConfig(ctx, state.BrokerHosts, LimitConnectionRate, state.InitialBrokerConfigValue, state.BrokerID, clusterConfig)
	if err != nil {
		return nil, err
	}
	return &action_kit_api.StopResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config %s back to initial value %d for broker node-id: %v", LimitConnectionRate, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil
}

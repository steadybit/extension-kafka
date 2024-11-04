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
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"strconv"
)

type AlterNumberNetworkThreadsAttack struct{}

type AlterNumberNetworkThreadsState struct {
	BrokerConfigValue        string
	BrokerID                 int32
	InitialBrokerConfigValue string
}

const (
	NumberNetworkThreads = "num.network.threads"
)

var _ action_kit_sdk.Action[AlterNumberNetworkThreadsState] = (*AlterNumberNetworkThreadsAttack)(nil)

func NewAlterNumberNetworkThreadsAttack() action_kit_sdk.Action[AlterNumberNetworkThreadsState] {
	return &AlterNumberNetworkThreadsAttack{}
}

func (k *AlterNumberNetworkThreadsAttack) NewEmptyState() AlterNumberNetworkThreadsState {
	return AlterNumberNetworkThreadsState{}
}

func (k *AlterNumberNetworkThreadsAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.network-threads", kafkaBrokerTargetId),
		Label:       "Alter Network Threads Number",
		Description: "Alter the number of network threads",
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
		TimeControl: action_kit_api.TimeControlExternal,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Label:        "Duration",
				Description:  extutil.Ptr("The duration of the action. The broker configuration will be reverted at the end of the action."),
				Name:         "duration",
				Type:         action_kit_api.Duration,
				DefaultValue: extutil.Ptr("60s"),
				Required:     extutil.Ptr(true),
			},
			{
				Label:        "Number of IO Threads",
				Description:  extutil.Ptr("Reduce the number of I/O threads to limit the broker’s capacity to perform disk operations, potentially causing increased latency or request timeouts."),
				Name:         "io_threads",
				Type:         action_kit_api.Integer,
				DefaultValue: extutil.Ptr("4"),
				Required:     extutil.Ptr(true),
			},
		},
	}
}

func (k *AlterNumberNetworkThreadsAttack) Prepare(_ context.Context, state *AlterNumberNetworkThreadsState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	state.BrokerConfigValue = extutil.ToString(request.Config["max_bytes"])

	return nil, nil
}

func (k *AlterNumberNetworkThreadsAttack) Start(ctx context.Context, state *AlterNumberNetworkThreadsState) (*action_kit_api.StartResult, error) {
	adminClient, err := CreateNewAdminClient()
	if err != nil {
		return nil, err
	}

	// Get the initial value
	configs, err := adminClient.DescribeBrokerConfigs(ctx, state.BrokerID)
	if err != nil {
		return nil, err
	}
	_, err = configs.On(strconv.FormatInt(int64(state.BrokerID), 10), func(resourceConfig *kadm.ResourceConfig) error {

		for i := range resourceConfig.Configs {
			if resourceConfig.Configs[i].Key == NumberNetworkThreads {
				state.InitialBrokerConfigValue = resourceConfig.Configs[i].MaybeValue()
				// Found!
				break
			}
		}

		return err
	})
	if err != nil {
		return nil, err
	}
	if state.InitialBrokerConfigValue == "" {
		log.Warn().Msgf("No initial value found for configuration key: "+NumberNetworkThreads+", for broker node-id: %d", state.BrokerID)
	}

	// If initial value is retrieved without errors, proceed with alter config
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: NumberNetworkThreads, Value: extutil.Ptr(state.BrokerConfigValue)}}, state.BrokerID)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, response := range responses {
		if response.Err != nil {
			detailedError := errors.New(response.Err.Error() + " Response from Broker: " + response.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &action_kit_api.StartResult{
		Messages: &[]action_kit_api.Message{{
			Level:   extutil.Ptr(action_kit_api.Info),
			Message: fmt.Sprintf("Alter config max.connection.creation.rate with value %s (initial value was: %s) for broker node-id: %v", state.BrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil

}

func (k *AlterNumberNetworkThreadsAttack) Stop(ctx context.Context, state *AlterNumberNetworkThreadsState) (*action_kit_api.StopResult, error) {
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

	// If initial value is retrieved without errors, proceed with alter config
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: "max.connection.creation.rate", Value: extutil.Ptr(state.InitialBrokerConfigValue)}}, state.BrokerID)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, response := range responses {
		if response.Err != nil {
			detailedError := errors.New(response.Err.Error() + " Response from Broker: " + response.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return nil, nil
}

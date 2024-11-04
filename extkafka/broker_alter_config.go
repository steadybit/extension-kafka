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
	"strings"
)

type KafkaBrokerAlterConfigAttack struct{}

type KafkaAlterConfigState struct {
	BrokerConfigKey          string
	BrokerConfigValue        string
	BrokerID                 int32
	InitialBrokerConfigValue string
}

var _ action_kit_sdk.Action[KafkaAlterConfigState] = (*KafkaBrokerAlterConfigAttack)(nil)

func NewKafkaBrokerAlterConfigAttack() action_kit_sdk.Action[KafkaAlterConfigState] {
	return &KafkaBrokerAlterConfigAttack{}
}

func (k *KafkaBrokerAlterConfigAttack) NewEmptyState() KafkaAlterConfigState {
	return KafkaAlterConfigState{}
}

func (k *KafkaBrokerAlterConfigAttack) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.alter-config", kafkaBrokerTargetId),
		Label:       "Alter Broker Config",
		Description: "Alter the configuration of one or multiple broker",
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
				DefaultValue: extutil.Ptr("180s"),
				Required:     extutil.Ptr(true),
			},
			{
				Name:        "brokerConfigs",
				Label:       "Broker Configs to alter",
				Description: extutil.Ptr("Different examples of broker configs to alter"),
				Required:    extutil.Ptr(true),
				Type:        action_kit_api.String,
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ExplicitParameterOption{
						Label: "Reduce network threads to 2 to simulate network bottlenecks",
						Value: "num.network.threads=2",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Reduce the number of I/O threads to 4 to limit the broker’s capacity to perform disk operations, potentially causing increased latency or request timeouts.",
						Value: "num.io.threads=4",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Limit the connection creation rate at 10 to simulate slow acceptance of new connections",
						Value: "max.connection.creation.rate=10",
					},
					action_kit_api.ExplicitParameterOption{
						Label: "Set a very low max bytes per message to 100 kilobytes to simulate message size rejections",
						Value: "message.max.bytes=100",
					},
				}),
			},
		},
	}
}

func (k *KafkaBrokerAlterConfigAttack) Prepare(_ context.Context, state *KafkaAlterConfigState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	var err error
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	if _, ok := request.Config["brokerConfigs"]; ok {
		state.BrokerConfigKey = strings.Split(extutil.ToString(request.Config["brokerConfigs"]), "=")[0]
		state.BrokerConfigValue = strings.Split(extutil.ToString(request.Config["brokerConfigs"]), "=")[1]
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse broker configurations")
			return nil, err
		}
	}

	return nil, nil
}

func (k *KafkaBrokerAlterConfigAttack) Start(ctx context.Context, state *KafkaAlterConfigState) (*action_kit_api.StartResult, error) {
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
			if resourceConfig.Configs[i].Key == state.BrokerConfigKey {
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
		log.Warn().Msgf("No initial value found for configuration key: %s, for broker node-id: %d", state.BrokerConfigKey, state.BrokerID)
	}

	// If initial value is retrieved without errors, proceed with alter config
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: state.BrokerConfigKey, Value: extutil.Ptr(state.BrokerConfigValue)}}, state.BrokerID)
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
			Message: fmt.Sprintf("Alter config %s with value %s (initial value was: %s) for broker node-id: %v", state.BrokerConfigKey, state.BrokerConfigValue, state.InitialBrokerConfigValue, state.BrokerID),
		}},
	}, nil

}

func (k *KafkaBrokerAlterConfigAttack) Stop(ctx context.Context, state *KafkaAlterConfigState) (*action_kit_api.StopResult, error) {
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
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: state.BrokerConfigKey, Value: extutil.Ptr(state.InitialBrokerConfigValue)}}, state.BrokerID)
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

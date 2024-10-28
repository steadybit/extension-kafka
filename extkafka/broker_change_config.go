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
)

type KafkaBrokerAlterConfigAttack struct{}

type KafkaAlterConfigState struct {
	BrokerConfigs map[string]string
	BrokerID      int32
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
		TimeControl: action_kit_api.TimeControlInstantaneous,
		Kind:        action_kit_api.Attack,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:        "brokerConfigs",
				Label:       "Broker Configs to alter",
				Description: extutil.Ptr("Broker configurations."),
				Type:        action_kit_api.KeyValue,
				Order:       extutil.Ptr(4),
			},
		},
	}
}

func (k *KafkaBrokerAlterConfigAttack) Prepare(ctx context.Context, state *KafkaAlterConfigState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	var err error
	state.BrokerID = extutil.ToInt32(request.Target.Attributes["kafka.broker.node-id"][0])
	if _, ok := request.Config["brokerConfigs"]; ok {
		state.BrokerConfigs, err = extutil.ToKeyValue(request.Config, "brokerConfigs")
		if err != nil {
			log.Error().Err(err).Msg("Failed to parse broker configurations")
			return nil, err
		}
	}

	return nil, nil
}

func (k *KafkaBrokerAlterConfigAttack) Start(ctx context.Context, state *KafkaAlterConfigState) (*action_kit_api.StartResult, error) {
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

	responses, err := adminClient.AlterBrokerConfigs(ctx, convertAlterConfig(state.BrokerConfigs), state.BrokerID)
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
			Message: fmt.Sprintf("Alter config %v for broker node-id: %v", state.BrokerConfigs, state.BrokerID),
		}},
	}, nil

}

func convertAlterConfig(brokerCfgs map[string]string) []kadm.AlterConfig {
	alterConfig := make([]kadm.AlterConfig, 0)

	for key, value := range brokerCfgs {
		alterConfig = append(alterConfig, kadm.AlterConfig{Name: key, Value: extutil.Ptr(value)})
	}

	return alterConfig
}

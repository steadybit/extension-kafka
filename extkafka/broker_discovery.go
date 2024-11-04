// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"time"
)

type kafkaBrokerDiscovery struct {
}

var (
	_ discovery_kit_sdk.TargetDescriber    = (*kafkaBrokerDiscovery)(nil)
	_ discovery_kit_sdk.AttributeDescriber = (*kafkaBrokerDiscovery)(nil)
)

func NewKafkaBrokerDiscovery(ctx context.Context) discovery_kit_sdk.TargetDiscovery {
	discovery := &kafkaBrokerDiscovery{}
	return discovery_kit_sdk.NewCachedTargetDiscovery(discovery,
		discovery_kit_sdk.WithRefreshTargetsNow(),
		discovery_kit_sdk.WithRefreshTargetsInterval(ctx, time.Duration(config.Config.DiscoveryIntervalKafkaBroker)*time.Second),
	)
}

func (r *kafkaBrokerDiscovery) Describe() discovery_kit_api.DiscoveryDescription {
	return discovery_kit_api.DiscoveryDescription{
		Id: kafkaBrokerTargetId,
		Discover: discovery_kit_api.DescribingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr(fmt.Sprintf("%ds", config.Config.DiscoveryIntervalKafkaBroker)),
		},
	}
}

func (r *kafkaBrokerDiscovery) DescribeTarget() discovery_kit_api.TargetDescription {
	return discovery_kit_api.TargetDescription{
		Id:       kafkaBrokerTargetId,
		Label:    discovery_kit_api.PluralLabel{One: "Kafka broker", Other: "Kafka brokers"},
		Category: extutil.Ptr("kafka"),
		Version:  extbuild.GetSemverVersionStringOrUnknown(),
		Icon:     extutil.Ptr(kafkaIcon),
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "kafka.broker.node-id"},
				{Attribute: "kafka.broker.host"},
				{Attribute: "kafka.broker.port"},
				{Attribute: "kafka.broker.rack"},
			},
			OrderBy: []discovery_kit_api.OrderBy{
				{
					Attribute: "steadybit.label",
					Direction: "ASC",
				},
			},
		},
	}
}

func (r *kafkaBrokerDiscovery) DescribeAttributes() []discovery_kit_api.AttributeDescription {
	return []discovery_kit_api.AttributeDescription{
		{
			Attribute: "kafka.cluster.name",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka cluster name",
				Other: "Kafka cluster names",
			},
		},
		{
			Attribute: "kafka.broker.node-id",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka broker node id",
				Other: "Kafka broker node ids",
			},
		}, {
			Attribute: "kafka.broker.host",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka broker host",
				Other: "Kafka broker hosts",
			},
		},
		{
			Attribute: "kafka.broker.port",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka broker port",
				Other: "Kafka broker ports",
			},
		},
		{
			Attribute: "kafka.broker.rack",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka broker rack",
				Other: "Kafka broker racks",
			},
		},
	}
}

func (r *kafkaBrokerDiscovery) DiscoverTargets(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllBrokers(ctx)
}

func getAllBrokers(ctx context.Context) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

	client, err := CreateNewAdminClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Create topic "franz-go" if it doesn't exist already
	brokerDetails, err := client.ListBrokers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %v", err)
	}
	for _, broker := range brokerDetails {
		result = append(result, toBrokerTarget(broker))
	}

	return result, nil
}

func toBrokerTarget(broker kadm.BrokerDetail) discovery_kit_api.Target {
	id := fmt.Sprintf("%v", broker.NodeID)
	label := broker.Host

	attributes := make(map[string][]string)
	attributes["kafka.broker.node-id"] = []string{fmt.Sprintf("%v", broker.NodeID)}
	attributes["kafka.broker.host"] = []string{label}
	attributes["kafka.broker.port"] = []string{fmt.Sprintf("%v", broker.Port)}
	if broker.Rack != nil {
		attributes["kafka.broker.rack"] = []string{*broker.Rack}
	}

	return discovery_kit_api.Target{
		Id:         id,
		Label:      label,
		TargetType: kafkaBrokerTargetId,
		Attributes: attributes,
	}
}

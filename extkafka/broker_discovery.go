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
		discovery_kit_sdk.WithRefreshTargetsInterval(ctx, time.Duration(config.Config.DiscoveryIntervalKafka)*time.Second),
	)
}

func (r *kafkaBrokerDiscovery) Describe() discovery_kit_api.DiscoveryDescription {
	return discovery_kit_api.DiscoveryDescription{
		Id: kafkaBrokerTargetId,
		Discover: discovery_kit_api.DescribingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr(fmt.Sprintf("%ds", config.Config.DiscoveryIntervalKafka)),
		},
	}
}

func (r *kafkaBrokerDiscovery) DescribeTarget() discovery_kit_api.TargetDescription {
	return discovery_kit_api.TargetDescription{
		Id:       kafkaBrokerTargetId,
		Label:    discovery_kit_api.PluralLabel{One: "MSK broker", Other: "MSK brokers"},
		Category: extutil.Ptr("cloud"),
		Version:  extbuild.GetSemverVersionStringOrUnknown(),
		Icon:     extutil.Ptr(kafkaIcon),
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "aws.msk.cluster.state"},
				{Attribute: "aws.msk.cluster.version"},
				{Attribute: "aws.msk.cluster.broker.kafka-version"},
				{Attribute: "aws.account"},
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

	admin := kadm.NewClient(KafkaClient)
	defer admin.Close()

	// Fetch cluster metadata
	//KafkaClient.ForceMetadataRefresh()
	metadata, err := admin.Metadata(ctx)
	if err != nil {
		return nil, err
	}

	clusterName := metadata.Cluster

	for _, broker := range metadata.Brokers {
		result = append(result, toBrokerTarget(broker, clusterName))
	}

	return result, nil
}

func toBrokerTarget(broker kadm.BrokerDetail, clusterName string) discovery_kit_api.Target {
	attributes := make(map[string][]string)
	attributes["kafka.cluster.name"] = []string{clusterName}
	attributes["kafka.broker.node-id"] = []string{fmt.Sprintf("%v", broker.NodeID)}
	attributes["kafka.broker.host"] = []string{broker.Host}
	attributes["kafka.broker.port"] = []string{fmt.Sprintf("%v", broker.Port)}
	attributes["kafka.broker.rack"] = []string{*broker.Rack}

	label := clusterName + "-" + fmt.Sprintf("%v", broker.NodeID)

	return discovery_kit_api.Target{
		Id:         label,
		Label:      label,
		TargetType: kafkaBrokerTargetId,
		Attributes: attributes,
	}
}

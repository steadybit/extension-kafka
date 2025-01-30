// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_commons"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"strings"
	"time"
)

type kafkaBrokerDiscovery struct {
}

var (
	_ discovery_kit_sdk.TargetDescriber          = (*kafkaBrokerDiscovery)(nil)
	_ discovery_kit_sdk.AttributeDescriber       = (*kafkaBrokerDiscovery)(nil)
	_ discovery_kit_sdk.EnrichmentRulesDescriber = (*kafkaBrokerDiscovery)(nil)
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
				{Attribute: "kafka.broker.is-controller"},
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

func (r *kafkaBrokerDiscovery) DescribeEnrichmentRules() []discovery_kit_api.TargetEnrichmentRule {
	return []discovery_kit_api.TargetEnrichmentRule{
		getBrokerToPodEnrichmentRule(),
		getBrokerToContainerEnrichmentRule(),
	}
}

func getBrokerToPodEnrichmentRule() discovery_kit_api.TargetEnrichmentRule {
	return discovery_kit_api.TargetEnrichmentRule{
		Id:      "com.steadybit.extension_kafka.kafka-broker-to-pod",
		Version: extbuild.GetSemverVersionStringOrUnknown(),
		Src: discovery_kit_api.SourceOrDestination{
			Type: kafkaBrokerTargetId,
			Selector: map[string]string{
				"kafka.pod.name":      "${dest.k8s.pod.name}",
				"kafka.pod.namespace": "${dest.k8s.namespace}",
			},
		},
		Dest: discovery_kit_api.SourceOrDestination{
			Type: "com.steadybit.extension_kubernetes.kubernetes-pod",
			Selector: map[string]string{
				"k8s.pod.name":  "${src.kafka.pod.name}",
				"k8s.namespace": "${src.kafka.pod.namespace}",
			},
		},
		Attributes: []discovery_kit_api.Attribute{
			{
				Matcher: discovery_kit_api.Equals,
				Name:    "kafka.broker.node-id",
			},
			{
				Matcher: discovery_kit_api.Equals,
				Name:    "kafka.broker.is-controller",
			},
		},
	}
}

func getBrokerToContainerEnrichmentRule() discovery_kit_api.TargetEnrichmentRule {
	return discovery_kit_api.TargetEnrichmentRule{
		Id:      "com.steadybit.extension_kafka.kafka-broker-to-container",
		Version: extbuild.GetSemverVersionStringOrUnknown(),
		Src: discovery_kit_api.SourceOrDestination{
			Type: kafkaBrokerTargetId,
			Selector: map[string]string{
				"kafka.pod.name":      "${dest.k8s.pod.name}",
				"kafka.pod.namespace": "${dest.k8s.namespace}",
			},
		},
		Dest: discovery_kit_api.SourceOrDestination{
			Type: "com.steadybit.extension_container.container",
			Selector: map[string]string{
				"k8s.pod.name":  "${src.kafka.pod.name}",
				"k8s.namespace": "${src.kafka.pod.namespace}",
			},
		},
		Attributes: []discovery_kit_api.Attribute{
			{
				Matcher: discovery_kit_api.Equals,
				Name:    "kafka.broker.node-id",
			},
			{
				Matcher: discovery_kit_api.Equals,
				Name:    "kafka.broker.is-controller",
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
		},
		{
			Attribute: "kafka.broker.is-controller",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka broker controller",
				Other: "Kafka broker controller",
			},
		},
		{
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
		{
			Attribute: "kafka.pod.name",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka pod name",
				Other: "Kafka pod names",
			},
		},
		{
			Attribute: "kafka.pod.namespace",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka pod namespace",
				Other: "Kafka pod namespaces",
			},
		},
	}
}

func (r *kafkaBrokerDiscovery) DiscoverTargets(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllBrokers(ctx)
}

func getAllBrokers(ctx context.Context) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

	client, err := createNewAdminClient(strings.Split(config.Config.SeedBrokers, ","))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Create topic "franz-go" if it doesn't exist already
	brokerDetails, err := client.ListBrokers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list brokers: %v", err)
	}
	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get brokers metadata : %v", err)
	}
	for _, broker := range brokerDetails {
		result = append(result, toBrokerTarget(broker, metadata.Controller))
	}

	return discovery_kit_commons.ApplyAttributeExcludes(result, config.Config.DiscoveryAttributesExcludesBrokers), nil
}

func toBrokerTarget(broker kadm.BrokerDetail, controller int32) discovery_kit_api.Target {
	id := fmt.Sprintf("%v", broker.NodeID)
	label := broker.Host

	attributes := make(map[string][]string)
	attributes["kafka.broker.node-id"] = []string{fmt.Sprintf("%v", broker.NodeID)}
	attributes["kafka.broker.is-controller"] = []string{"false"}
	if broker.NodeID == controller {
		attributes["kafka.broker.is-controller"] = []string{"true"}
	}
	attributes["kafka.broker.host"] = []string{label}
	attributes["kafka.broker.port"] = []string{fmt.Sprintf("%v", broker.Port)}
	if broker.Rack != nil {
		attributes["kafka.broker.rack"] = []string{*broker.Rack}
	}
	if len(strings.Split(broker.Host, ".")) == 4 && strings.HasSuffix(broker.Host, ".svc") {
		podName := strings.Split(broker.Host, ".")[0]
		namespace := strings.Split(broker.Host, ".")[2]

		attributes["kafka.pod.name"] = []string{podName}
		attributes["kafka.pod.namespace"] = []string{namespace}
	}

	return discovery_kit_api.Target{
		Id:         id,
		Label:      label,
		TargetType: kafkaBrokerTargetId,
		Attributes: attributes,
	}
}

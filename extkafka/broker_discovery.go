// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_commons"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"strconv"
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
		Label:    discovery_kit_api.PluralLabel{One: "Kafka Broker", Other: "Kafka Brokers"},
		Category: extutil.Ptr("kafka"),
		Version:  extbuild.GetSemverVersionStringOrUnknown(),
		Icon:     extutil.Ptr(kafkaIcon),
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "kafka.broker.cluster-name"},
				{Attribute: "kafka.broker.node-id"},
				{Attribute: "kafka.broker.is-controller"},
				{Attribute: "kafka.broker.host"},
				{Attribute: "kafka.broker.port"},
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
	return getAllBrokersMultiCluster(ctx)
}

func getAllBrokersMultiCluster(ctx context.Context) ([]discovery_kit_api.Target, error) {
	clusters := config.GetAllClusterConfigs()

	type clusterResult struct {
		targets []discovery_kit_api.Target
		err     error
	}

	resultChan := make(chan clusterResult, len(clusters))

	// Discover from all clusters in parallel
	for clusterName, clusterConfig := range clusters {
		go func(name string, cfg *config.ClusterConfig) {
			targets, err := discoverBrokersForCluster(ctx, name, cfg)
			resultChan <- clusterResult{targets: targets, err: err}
		}(clusterName, clusterConfig)
	}

	// Collect results
	allTargets := make([]discovery_kit_api.Target, 0, 20*len(clusters))
	var errors []error

	for i := 0; i < len(clusters); i++ {
		result := <-resultChan
		if result.err != nil {
			log.Warn().Err(result.err).Msg("Failed to discover brokers from cluster")
			errors = append(errors, result.err)
		} else {
			allTargets = append(allTargets, result.targets...)
		}
	}

	// Fail only if all clusters failed
	if len(errors) == len(clusters) && len(clusters) > 0 {
		return nil, fmt.Errorf("failed to discover from all clusters: %v", errors)
	}

	return discovery_kit_commons.ApplyAttributeExcludes(allTargets, config.Config.DiscoveryAttributesExcludesBrokers), nil
}

func discoverBrokersForCluster(ctx context.Context, clusterName string, clusterConfig *config.ClusterConfig) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

	client, err := createNewAdminClientWithConfig(strings.Split(clusterConfig.SeedBrokers, ","), clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client for cluster %s: %s", clusterName, err.Error())
	}
	defer client.Close()

	brokerDetails, err := client.ListBrokers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list brokers for cluster %s: %v", clusterName, err)
	}

	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get brokers metadata for cluster %s: %v", clusterName, err)
	}

	log.Debug().Msgf("Cluster %s: Number of brokers discovered: %d", clusterName, len(brokerDetails))
	log.Debug().Msgf("Cluster %s: Node IDs discovered: %v", clusterName, brokerDetails.NodeIDs())

	for _, broker := range brokerDetails {
		result = append(result, toBrokerTarget(broker, metadata.Controller, metadata.Cluster))
	}

	return result, nil
}

// Keep for backward compatibility (used in tests or other places)
func getAllBrokers(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllBrokersMultiCluster(ctx)
}

func toBrokerTarget(broker kadm.BrokerDetail, controller int32, clusterName string) discovery_kit_api.Target {
	id := broker.Host + "-" + strconv.Itoa(int(broker.Port))
	label := broker.Host

	attributes := make(map[string][]string)
	attributes["kafka.cluster.name"] = []string{clusterName}
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

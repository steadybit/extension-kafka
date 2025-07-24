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
	"strconv"
	"strings"
	"time"
)

type kafkaTopicDiscovery struct {
}

var (
	_ discovery_kit_sdk.TargetDescriber    = (*kafkaTopicDiscovery)(nil)
	_ discovery_kit_sdk.AttributeDescriber = (*kafkaTopicDiscovery)(nil)
)

func NewKafkaTopicDiscovery(ctx context.Context) discovery_kit_sdk.TargetDiscovery {
	discovery := &kafkaTopicDiscovery{}
	return discovery_kit_sdk.NewCachedTargetDiscovery(discovery,
		discovery_kit_sdk.WithRefreshTargetsNow(),
		discovery_kit_sdk.WithRefreshTargetsInterval(ctx, time.Duration(config.Config.DiscoveryIntervalKafkaTopic)*time.Second),
	)
}

func (r *kafkaTopicDiscovery) Describe() discovery_kit_api.DiscoveryDescription {
	return discovery_kit_api.DiscoveryDescription{
		Id: kafkaTopicTargetId,
		Discover: discovery_kit_api.DescribingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr(fmt.Sprintf("%ds", config.Config.DiscoveryIntervalKafkaTopic)),
		},
	}
}

func (r *kafkaTopicDiscovery) DescribeTarget() discovery_kit_api.TargetDescription {
	return discovery_kit_api.TargetDescription{
		Id:       kafkaTopicTargetId,
		Label:    discovery_kit_api.PluralLabel{One: "Kafka topic", Other: "Kafka topics"},
		Category: extutil.Ptr("kafka"),
		Version:  extbuild.GetSemverVersionStringOrUnknown(),
		Icon:     extutil.Ptr(kafkaIcon),
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "kafka.topic.name"},
				{Attribute: "kafka.topic.partitions-leaders"},
				{Attribute: "kafka.topic.partitions-replicas"},
				{Attribute: "kafka.topic.partitions-isr"},
				{Attribute: "kafka.topic.replication-factor"},
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

func (r *kafkaTopicDiscovery) DescribeAttributes() []discovery_kit_api.AttributeDescription {
	return []discovery_kit_api.AttributeDescription{
		{
			Attribute: "kafka.topic.name",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic name",
				Other: "Kafka topic names",
			},
		},
		{
			Attribute: "kafka.topic.partitions",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic partitions",
				Other: "Kafka topic partitions",
			},
		},
		{
			Attribute: "kafka.topic.partitions-leaders",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic partitions leaders",
				Other: "Kafka topic partitions leaders",
			},
		},
		{
			Attribute: "kafka.topic.partitions-replicas",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic partitions replicas",
				Other: "Kafka topic partitions replicas",
			},
		},
		{
			Attribute: "kafka.topic.partitions-isr",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic partitions in-sync-replicas",
				Other: "Kafka topic partitions in-sync-replicas",
			},
		},
		{
			Attribute: "kafka.topic.replication-factor",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic replication factor",
				Other: "Kafka topic replication factors",
			},
		},
	}
}

func (r *kafkaTopicDiscovery) DiscoverTargets(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllTopics(ctx)
}

func getAllTopics(ctx context.Context) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

	client, err := createNewAdminClient(strings.Split(config.Config.SeedBrokers, ","))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	// Create topic "franz-go" if it doesn't exist already
	topicDetails, err := client.ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %v", err)
	}
	metadata, err := client.BrokerMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get brokers metadata : %v", err)
	}

	for _, t := range topicDetails {
		if !t.IsInternal {
			result = append(result, toTopicTarget(t, metadata.Cluster))
		}
	}

	return discovery_kit_commons.ApplyAttributeExcludes(result, config.Config.DiscoveryAttributesExcludesTopics), nil
}

func toTopicTarget(topic kadm.TopicDetail, clusterName string) discovery_kit_api.Target {
	label := topic.Topic

	partitions := make([]string, len(topic.Partitions))
	partitionsLeaders := make([]string, len(topic.Partitions))
	partitionsReplicas := make([]string, len(topic.Partitions))
	partitionsInSyncReplicas := make([]string, len(topic.Partitions))

	for i, partDetail := range topic.Partitions.Sorted() {
		partitions[i] = strconv.FormatInt(int64(partDetail.Partition), 10)
	}

	for i, partDetail := range topic.Partitions.Sorted() {
		partitionsLeaders[i] = fmt.Sprintf("%d->leader=%d", partDetail.Partition, partDetail.Leader)
	}

	for i, partDetail := range topic.Partitions.Sorted() {
		partitionsReplicas[i] = fmt.Sprintf("%d->replicas=%v", partDetail.Partition, partDetail.Replicas)
	}

	for i, partDetail := range topic.Partitions.Sorted() {
		partitionsInSyncReplicas[i] = fmt.Sprintf("%d->in-sync-replicas=%v", partDetail.Partition, partDetail.ISR)
	}

	attributes := make(map[string][]string)
	attributes["kafka.cluster.name"] = []string{clusterName}
	attributes["kafka.topic.name"] = []string{topic.Topic}
	attributes["kafka.topic.partitions"] = partitions
	attributes["kafka.topic.partitions-leaders"] = partitionsLeaders
	attributes["kafka.topic.partitions-replicas"] = partitionsReplicas
	attributes["kafka.topic.partitions-isr"] = partitionsInSyncReplicas
	attributes["kafka.topic.replication-factor"] = []string{fmt.Sprintf("%v", topic.Partitions.NumReplicas())}

	return discovery_kit_api.Target{
		Id:         fmt.Sprintf("%s-%s", label, clusterName),
		Label:      label,
		TargetType: kafkaTopicTargetId,
		Attributes: attributes,
	}
}

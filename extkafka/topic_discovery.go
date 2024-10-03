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
	"github.com/twmb/franz-go/pkg/kgo"
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
				{Attribute: "kafka.topic.id"},
				{Attribute: "kafka.topic.name"},
				{Attribute: "kafka.topic.internal"},
				{Attribute: "kafka.partitions.total"},
				{Attribute: "kafka.partitions.replicas"},
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
			Attribute: "kafka.topic.id",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic id",
				Other: "Kafka topic ids",
			},
		}, {
			Attribute: "kafka.topic.name",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic name",
				Other: "Kafka topic names",
			},
		},
		{
			Attribute: "kafka.partitions.total",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic partitions total",
				Other: "Kafka topic partitions totals",
			},
		},
		{
			Attribute: "kafka.partitions.replicas",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka topic replicas total",
				Other: "Kafka topic replicas totals",
			},
		},
	}
}

func (r *kafkaTopicDiscovery) DiscoverTargets(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllTopics(ctx)
}

func getAllTopics(ctx context.Context) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create topic "franz-go" if it doesn't exist already
	topicDetails, err := adminClient.ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %v", err)
	}

	for _, topic := range topicDetails {
		result = append(result, toTopicTarget(topic))
	}

	return result, nil
}

func toTopicTarget(topic kadm.TopicDetail) discovery_kit_api.Target {
	label := topic.Topic
	id := fmt.Sprintf("%v", topic.ID)

	attributes := make(map[string][]string)
	attributes["kafka.topic.name"] = []string{topic.Topic}
	attributes["kafka.topic.id"] = []string{fmt.Sprintf("%v", topic.ID)}
	attributes["kafka.partitions.total"] = []string{fmt.Sprintf("%v", topic.Partitions.Numbers())}
	attributes["kafka.partitions.replicas"] = []string{fmt.Sprintf("%v", topic.Partitions.NumReplicas())}

	return discovery_kit_api.Target{
		Id:         id,
		Label:      label,
		TargetType: kafkaTopicTargetId,
		Attributes: attributes,
	}
}

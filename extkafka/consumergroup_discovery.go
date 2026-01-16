// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_commons"
	"github.com/steadybit/discovery-kit/go/discovery_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
)

type kafkaConsumerGroupDiscovery struct {
}

var (
	_ discovery_kit_sdk.TargetDescriber    = (*kafkaConsumerGroupDiscovery)(nil)
	_ discovery_kit_sdk.AttributeDescriber = (*kafkaConsumerGroupDiscovery)(nil)
)

func NewKafkaConsumerGroupDiscovery(ctx context.Context) discovery_kit_sdk.TargetDiscovery {
	discovery := &kafkaConsumerGroupDiscovery{}
	return discovery_kit_sdk.NewCachedTargetDiscovery(discovery,
		discovery_kit_sdk.WithRefreshTargetsNow(),
		discovery_kit_sdk.WithRefreshTargetsInterval(ctx, time.Duration(config.Config.DiscoveryIntervalConsumerGroup)*time.Second),
	)
}

func (r *kafkaConsumerGroupDiscovery) Describe() discovery_kit_api.DiscoveryDescription {
	return discovery_kit_api.DiscoveryDescription{
		Id: kafkaConsumerTargetId,
		Discover: discovery_kit_api.DescribingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr(fmt.Sprintf("%ds", config.Config.DiscoveryIntervalConsumerGroup)),
		},
	}
}

func (r *kafkaConsumerGroupDiscovery) DescribeTarget() discovery_kit_api.TargetDescription {
	return discovery_kit_api.TargetDescription{
		Id:       kafkaConsumerTargetId,
		Label:    discovery_kit_api.PluralLabel{One: "Kafka Consumer Group", Other: "Kafka Consumer Groups"},
		Category: extutil.Ptr("kafka"),
		Version:  extbuild.GetSemverVersionStringOrUnknown(),
		Icon:     extutil.Ptr(kafkaIcon),
		Table: discovery_kit_api.Table{
			Columns: []discovery_kit_api.Column{
				{Attribute: "steadybit.label"},
				{Attribute: "kafka.consumer-group.coordinator"},
				{Attribute: "kafka.consumer-group.protocol-type"},
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

func (r *kafkaConsumerGroupDiscovery) DescribeAttributes() []discovery_kit_api.AttributeDescription {
	return []discovery_kit_api.AttributeDescription{
		{
			Attribute: "kafka.consumer-group.name",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka consumer group name",
				Other: "Kafka consumer group names",
			},
		},
		{
			Attribute: "kafka.consumer-group.coordinator",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka consumer group coordinator",
				Other: "Kafka consumer group coordinators",
			},
		}, {
			Attribute: "kafka.consumer-group.protocol-type",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka consumer group protocol type",
				Other: "Kafka consumer group protocol types",
			},
		},
		{
			Attribute: "kafka.consumer-group.topics",
			Label: discovery_kit_api.PluralLabel{
				One:   "Kafka consumer group topic",
				Other: "Kafka consumer group topics",
			},
		},
	}
}

func (r *kafkaConsumerGroupDiscovery) DiscoverTargets(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllConsumerGroupsMultiCluster(ctx)
}

func getAllConsumerGroupsMultiCluster(ctx context.Context) ([]discovery_kit_api.Target, error) {
	clusters := config.GetAllClusterConfigs()

	type clusterResult struct {
		targets []discovery_kit_api.Target
		err     error
	}

	resultChan := make(chan clusterResult, len(clusters))

	// Discover from all clusters in parallel
	for clusterName, clusterConfig := range clusters {
		go func(name string, cfg *config.ClusterConfig) {
			targets, err := discoverConsumerGroupsForCluster(ctx, name, cfg)
			resultChan <- clusterResult{targets: targets, err: err}
		}(clusterName, clusterConfig)
	}

	// Collect results
	allTargets := make([]discovery_kit_api.Target, 0, 20*len(clusters))
	var errorList []error

	for i := 0; i < len(clusters); i++ {
		result := <-resultChan
		if result.err != nil {
			errorList = append(errorList, result.err)
		} else {
			allTargets = append(allTargets, result.targets...)
		}
	}

	// Fail only if all clusters failed
	if len(errorList) == len(clusters) && len(clusters) > 0 {
		return nil, fmt.Errorf("failed to discover from all clusters: %v", errorList)
	}

	return discovery_kit_commons.ApplyAttributeExcludes(allTargets, config.Config.DiscoveryAttributesExcludesConsumerGroups), nil
}

func discoverConsumerGroupsForCluster(ctx context.Context, clusterName string, clusterConfig *config.ClusterConfig) ([]discovery_kit_api.Target, error) {
	result := make([]discovery_kit_api.Target, 0, 20)

	client, err := createNewAdminClientWithConfig(strings.Split(clusterConfig.SeedBrokers, ","), clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client for cluster %s: %s", clusterName, err.Error())
	}
	defer client.Close()

	var seList *kadm.ShardErrors
	describedGroups, err := client.DescribeGroups(ctx)
	switch {
	case err == nil:
	case errors.As(err, &seList):
	default:
		return nil, fmt.Errorf("failed to describe consumer groups for cluster %s: %v", clusterName, err)
	}

	for _, group := range describedGroups.Sorted() {
		result = append(result, toConsumerGroupTarget(group, clusterName))
	}

	return result, nil
}

// Keep for backward compatibility
func getAllConsumerGroups(ctx context.Context) ([]discovery_kit_api.Target, error) {
	return getAllConsumerGroupsMultiCluster(ctx)
}

func toConsumerGroupTarget(group kadm.DescribedGroup, clusterName string) discovery_kit_api.Target {
	id := fmt.Sprintf("%v-%s", group.Group, clusterName)
	label := fmt.Sprintf("%v", group.Group)

	attributes := make(map[string][]string)
	attributes["kafka.cluster.name"] = []string{clusterName}
	attributes["kafka.consumer-group.name"] = []string{fmt.Sprintf("%v", group.Group)}
	attributes["kafka.consumer-group.coordinator"] = []string{fmt.Sprintf("%v", group.Coordinator.Host)}
	attributes["kafka.consumer-group.protocol-type"] = []string{group.ProtocolType}
	attributes["kafka.consumer-group.topics"] = group.AssignedPartitions().Topics()

	return discovery_kit_api.Target{
		Id:         id,
		Label:      label,
		TargetType: kafkaConsumerTargetId,
		Attributes: attributes,
	}
}

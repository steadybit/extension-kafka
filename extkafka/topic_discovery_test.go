// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"fmt"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"

	"context"
	"github.com/steadybit/extension-kafka/config"
	"github.com/twmb/franz-go/pkg/kfake"
	"strings"
)

func TestDescribeTopic(t *testing.T) {
	desc := (&kafkaTopicDiscovery{}).Describe()
	require.Equal(t, kafkaTopicTargetId, desc.Id)
	require.NotNil(t, desc.Discover.CallInterval)
}

func TestDescribeTargetTopic(t *testing.T) {
	d := &kafkaTopicDiscovery{}
	td := d.DescribeTarget()

	require.Equal(t, kafkaTopicTargetId, td.Id)
	require.Equal(t, "Kafka topic", td.Label.One)
	require.Equal(t, "Kafka topics", td.Label.Other)
	require.NotNil(t, td.Category)
	require.Equal(t, "kafka", *td.Category)
	require.Len(t, td.Table.Columns, 6)
	require.Len(t, td.Table.OrderBy, 1)

	ob := td.Table.OrderBy[0]
	require.Equal(t, "steadybit.label", ob.Attribute)
	require.Equal(t, discovery_kit_api.OrderByDirection("ASC"), ob.Direction)
}

func TestDescribeAttributesTopic(t *testing.T) {
	attrs := (&kafkaTopicDiscovery{}).DescribeAttributes()
	expected := []string{
		"kafka.topic.name",
		"kafka.topic.partitions",
		"kafka.topic.partitions-leaders",
		"kafka.topic.partitions-replicas",
		"kafka.topic.partitions-isr",
		"kafka.topic.replication-factor",
	}

	require.Len(t, attrs, len(expected))
	for _, want := range expected {
		found := false
		for _, a := range attrs {
			if a.Attribute == want {
				found = true
				break
			}
		}
		assert.Truef(t, found, "DescribeAttributes() missing %q", want)
	}
}

func TestToTopicTargetTopic(t *testing.T) {
	td := kadm.TopicDetail{
		Topic: "my-topic",
		Partitions: kadm.PartitionDetails{
			1: {Partition: 1, Leader: 101, Replicas: []int32{101, 102}, ISR: []int32{101}},
			0: {Partition: 0, Leader: 100, Replicas: []int32{100, 102}, ISR: []int32{100, 102}},
		},
	}
	cluster := "cluster-42"
	tgt := toTopicTarget(td, cluster)

	// Basic fields
	assert.Equal(t, "my-topic-cluster-42", tgt.Id)
	assert.Equal(t, "my-topic", tgt.Label)
	assert.Equal(t, kafkaTopicTargetId, tgt.TargetType)

	// Attributes
	attr := tgt.Attributes
	check := func(key string, want []string) {
		v, ok := attr[key]
		assert.True(t, ok, "missing attribute %q", key)
		assert.True(t, reflect.DeepEqual(v, want), "%s = %v; want %v", key, v, want)
	}

	check("kafka.cluster.name", []string{cluster})
	check("kafka.topic.name", []string{"my-topic"})
	check("kafka.topic.partitions", []string{"0", "1"})
	check("kafka.topic.partitions-leaders", []string{"0->leader=100", "1->leader=101"})
	check(
		"kafka.topic.partitions-replicas",
		[]string{
			fmt.Sprintf("0->replicas=%v", []int{100, 102}),
			fmt.Sprintf("1->replicas=%v", []int{101, 102}),
		},
	)
	check(
		"kafka.topic.partitions-isr",
		[]string{
			fmt.Sprintf("0->in-sync-replicas=%v", []int{100, 102}),
			fmt.Sprintf("1->in-sync-replicas=%v", []int{101}),
		},
	)
	check("kafka.topic.replication-factor", []string{"2"})
}

// TestDiscoverTargetsClusterName verifies that the kafka.cluster.name attribute
// is correctly set when discovering topics against a fake Kafka cluster.
func TestDiscoverTopicTargetsClusterName(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(1),
		kfake.ClusterID("test"),
	)
	require.NoError(t, err)
	defer c.Close()

	// Configure seed brokers for discovery
	seeds := c.ListenAddrs()
	config.Config.SeedBrokers = strings.Join(seeds, ",")

	// Ensure no excluded attributes
	config.Config.DiscoveryAttributesExcludesTopics = nil

	// Discover targets
	ctx := context.Background()
	targets, err := getAllTopics(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, targets)

	// Retrieve expected cluster name from metadata
	client, err := createNewAdminClient(strings.Split(config.Config.SeedBrokers, ","))
	require.NoError(t, err)
	defer client.Close()

	meta, err := client.BrokerMetadata(ctx)
	require.NoError(t, err)
	expected := meta.Cluster

	// Assert each discovered target has the correct cluster name attribute
	for _, tgt := range targets {
		values, ok := tgt.Attributes["kafka.cluster.name"]
		require.True(t, ok, "missing kafka.cluster.name for target %s", tgt.Id)
		require.Len(t, values, 1)
		require.Equal(t, expected, values[0])
	}
}

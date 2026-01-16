// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"fmt"
	"testing"

	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twmb/franz-go/pkg/kadm"

	"context"
	"strings"

	"github.com/steadybit/extension-kafka/config"
	"github.com/twmb/franz-go/pkg/kfake"
)

func TestDescribe(t *testing.T) {
	desc := (&kafkaTopicDiscovery{}).Describe()
	assert.Equal(t, kafkaTopicTargetId, desc.Id)
	assert.NotNil(t, desc.Discover.CallInterval)
}

func TestDescribeTarget(t *testing.T) {
	d := &kafkaTopicDiscovery{}
	td := d.DescribeTarget()
	assert.Equal(t, kafkaTopicTargetId, td.Id)
	assert.Equal(t, "Kafka Topic", td.Label.One)
	assert.Equal(t, "Kafka Topics", td.Label.Other)
	assert.Equal(t, "kafka", *td.Category)
	assert.Len(t, td.Table.Columns, 6)
	require.Len(t, td.Table.OrderBy, 1)
	assert.Equal(t, "steadybit.label", td.Table.OrderBy[0].Attribute)
	assert.Equal(t, discovery_kit_api.OrderByDirection("ASC"), td.Table.OrderBy[0].Direction)
}

func TestDescribeAttributes(t *testing.T) {
	d := &kafkaTopicDiscovery{}
	attrs := d.DescribeAttributes()
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

func TestToTopicTarget(t *testing.T) {
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
		assert.Equal(t, want, v)
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
		})
	check(
		"kafka.topic.partitions-isr",
		[]string{
			fmt.Sprintf("0->in-sync-replicas=%v", []int{100, 102}),
			fmt.Sprintf("1->in-sync-replicas=%v", []int{101}),
		})
	check("kafka.topic.replication-factor", []string{"2"})
}

// TestDiscoverTargetsClusterName verifies that the kafka.cluster.name attribute
// is correctly set when discovering topics against a fake Kafka cluster.
func TestDiscoverTargetsClusterName(t *testing.T) {
	// Set up fake Kafka cluster with a topic "steadybit"
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(1),
		kfake.ClusterID("test"),
	)
	require.NoError(t, err)
	defer c.Close()

	// Configure seed brokers for discovery
	seeds := c.ListenAddrs()
	seedBrokers := strings.Join(seeds, ",")

	// Initialize cluster configuration for test
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {
			SeedBrokers: seedBrokers,
		},
	})

	// Ensure no excluded attributes
	config.Config.DiscoveryAttributesExcludesTopics = nil

	// Discover targets using multi-cluster discovery
	ctx := context.Background()
	targets, err := getAllTopicsMultiCluster(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, targets)

	// Retrieve expected cluster name from metadata
	clusterConfig, err := config.GetClusterConfig("test-cluster")
	require.NoError(t, err)
	client, err := createNewAdminClientWithConfig(strings.Split(clusterConfig.SeedBrokers, ","), clusterConfig)
	require.NoError(t, err)
	defer client.Close()
	meta, err := client.BrokerMetadata(ctx)
	require.NoError(t, err)
	expected := meta.Cluster

	// Assert each discovered target has the correct cluster name attribute
	for _, tgt := range targets {
		values, ok := tgt.Attributes["kafka.cluster.name"]
		require.True(t, ok, "missing kafka.cluster.name for target %s", tgt.Id)
		require.Equal(t, []string{expected}, values)
	}
}

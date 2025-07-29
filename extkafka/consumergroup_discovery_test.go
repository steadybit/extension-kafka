// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package extkafka

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/twmb/franz-go/pkg/kadm"

	"context"
	"github.com/steadybit/extension-kafka/config"
	"github.com/twmb/franz-go/pkg/kfake"
	"strings"
)

// Test Describe()
func TestDescribe(t *testing.T) {
	d := &kafkaTopicDiscovery{}
	desc := d.Describe()
	if desc.Id != kafkaTopicTargetId {
		t.Errorf("Describe().Id = %q; want %q", desc.Id, kafkaTopicTargetId)
	}
	if desc.Discover.CallInterval == nil {
		t.Error("Describe().Discover.CallInterval = nil; want non-nil")
	}
}

// Test DescribeTarget()
func TestDescribeTarget(t *testing.T) {
	d := &kafkaTopicDiscovery{}
	td := d.DescribeTarget()

	if td.Id != kafkaTopicTargetId {
		t.Errorf("DescribeTarget().Id = %q; want %q", td.Id, kafkaTopicTargetId)
	}
	if td.Label.One != "Kafka topic" || td.Label.Other != "Kafka topics" {
		t.Errorf("DescribeTarget().Label = %+v; want One=\"Kafka topic\", Other=\"Kafka topics\"", td.Label)
	}
	if td.Category == nil || *td.Category != "kafka" {
		t.Errorf("DescribeTarget().Category = %v; want pointer to \"kafka\"", td.Category)
	}
	if len(td.Table.Columns) != 6 {
		t.Errorf("DescribeTarget().Table.Columns length = %d; want 6", len(td.Table.Columns))
	}
	if len(td.Table.OrderBy) != 1 {
		t.Errorf("DescribeTarget().Table.OrderBy length = %d; want 1", len(td.Table.OrderBy))
	} else {
		ob := td.Table.OrderBy[0]
		if ob.Attribute != "steadybit.label" || ob.Direction != "ASC" {
			t.Errorf("DescribeTarget().OrderBy[0] = %+v; want Attribute=\"steadybit.label\", Direction=\"ASC\"", ob)
		}
	}
}

// Test DescribeAttributes()
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

	if len(attrs) != len(expected) {
		t.Fatalf("DescribeAttributes() len = %d; want %d", len(attrs), len(expected))
	}

	for _, want := range expected {
		found := false
		for _, a := range attrs {
			if a.Attribute == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DescribeAttributes() missing %q", want)
		}
	}
}

// Test toTopicTarget()
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
	if want := "my-topic-cluster-42"; tgt.Id != want {
		t.Errorf("Id = %q; want %q", tgt.Id, want)
	}
	if tgt.Label != "my-topic" {
		t.Errorf("Label = %q; want %q", tgt.Label, "my-topic")
	}
	if tgt.TargetType != kafkaTopicTargetId {
		t.Errorf("TargetType = %q; want %q", tgt.TargetType, kafkaTopicTargetId)
	}

	// Attributes
	attr := tgt.Attributes

	check := func(key string, want []string) {
		v, ok := attr[key]
		if !ok {
			t.Errorf("missing attribute %q", key)
			return
		}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("%s = %v; want %v", key, v, want)
		}
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
func TestDiscoverTargetsClusterName(t *testing.T) {
	// Set up fake Kafka cluster with a topic "steadybit"
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(1),
		kfake.ClusterID("test"),
	)
	if err != nil {
		t.Fatalf("failed to create fake cluster: %v", err)
	}
	defer c.Close()

	// Configure seed brokers for discovery
	seeds := c.ListenAddrs()
	config.Config.SeedBrokers = strings.Join(seeds, ",")

	// Ensure no excluded attributes
	config.Config.DiscoveryAttributesExcludesTopics = nil

	// Discover targets
	ctx := context.Background()
	targets, err := getAllTopics(ctx)
	if err != nil {
		t.Fatalf("getAllTopics error: %v", err)
	}
	if len(targets) == 0 {
		t.Fatal("expected at least one discovered topic")
	}

	// Retrieve expected cluster name from metadata
	client, err := createNewAdminClient(strings.Split(config.Config.SeedBrokers, ","))
	if err != nil {
		t.Fatalf("createNewAdminClient error: %v", err)
	}
	defer client.Close()
	meta, err := client.BrokerMetadata(ctx)
	if err != nil {
		t.Fatalf("BrokerMetadata error: %v", err)
	}
	expected := meta.Cluster

	// Assert each discovered target has the correct cluster name attribute
	for _, tgt := range targets {
		values, ok := tgt.Attributes["kafka.cluster.name"]
		if !ok {
			t.Errorf("missing kafka.cluster.name for target %s", tgt.Id)
			continue
		}
		if len(values) != 1 || values[0] != expected {
			t.Errorf("kafka.cluster.name = %v; want [%s]", values, expected)
		}
	}
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// el_newCluster starts a kfake cluster with one topic and registers it as cluster "test-cluster".
func el_newCluster(t *testing.T, topic string, partitions int32) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(kfake.SeedTopics(partitions, topic), kfake.NumBrokers(3))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {SeedBrokers: strings.Join(c.ListenAddrs(), ",")},
	})
	return c
}

// el_advertiseKeys makes the cluster advertise request keys kfake does not implement (ACLs, partition
// reassignment, leader election), so clients send them and control functions can answer. It answers
// ApiVersions with kfake's own response plus the given keys, each supporting versions 0..max.
func el_advertiseKeys(t *testing.T, c *kfake.Cluster, maxVersions map[int16]int16) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...))
	require.NoError(t, err)
	defer cl.Close()
	base, err := kmsg.NewPtrApiVersionsRequest().RequestWith(context.Background(), cl)
	require.NoError(t, err)
	for key, maxVersion := range maxVersions {
		k := kmsg.NewApiVersionsResponseApiKey()
		k.ApiKey = key
		k.MaxVersion = maxVersion
		base.ApiKeys = append(base.ApiKeys, k)
	}
	c.ControlKey(int16(kmsg.ApiVersions), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		resp := *base
		resp.Version = req.GetVersion()
		return &resp, nil, true
	})
}

// el_electFake answers AlterPartitionAssignments and ElectLeaders, records the requested replica order and
// returns electErr for every elected partition.
type el_electFake struct {
	mu         sync.Mutex
	reassigned map[string]map[int32][]int32
	elections  int
}

func el_installElectFake(c *kfake.Cluster, electErr *kerr.Error) *el_electFake {
	f := &el_electFake{reassigned: map[string]map[int32][]int32{}}
	c.ControlKey(int16(kmsg.AlterPartitionAssignments), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		r := req.(*kmsg.AlterPartitionAssignmentsRequest)
		resp := r.ResponseKind().(*kmsg.AlterPartitionAssignmentsResponse)
		resp.Version = r.Version
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, t := range r.Topics {
			rt := kmsg.NewAlterPartitionAssignmentsResponseTopic()
			rt.Topic = t.Topic
			for _, p := range t.Partitions {
				if f.reassigned[t.Topic] == nil {
					f.reassigned[t.Topic] = map[int32][]int32{}
				}
				f.reassigned[t.Topic][p.Partition] = p.Replicas
				rp := kmsg.NewAlterPartitionAssignmentsResponseTopicPartition()
				rp.Partition = p.Partition
				rt.Partitions = append(rt.Partitions, rp)
			}
			resp.Topics = append(resp.Topics, rt)
		}
		return resp, nil, true
	})
	c.ControlKey(int16(kmsg.ElectLeaders), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		r := req.(*kmsg.ElectLeadersRequest)
		resp := r.ResponseKind().(*kmsg.ElectLeadersResponse)
		resp.Version = r.Version
		f.mu.Lock()
		f.elections++
		f.mu.Unlock()
		for _, t := range r.Topics {
			rt := kmsg.NewElectLeadersResponseTopic()
			rt.Topic = t.Topic
			for _, p := range t.Partitions {
				rp := kmsg.NewElectLeadersResponseTopicPartition()
				rp.Partition = p
				if electErr != nil {
					rp.ErrorCode = electErr.Code
				}
				rt.Partitions = append(rt.Partitions, rp)
			}
			resp.Topics = append(resp.Topics, rt)
		}
		return resp, nil, true
	})
	return f
}

func el_prepareRequest(topic string, partition any) action_kit_api.PrepareActionRequestBody {
	return extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target: &action_kit_api.Target{
			Attributes: map[string][]string{
				"kafka.cluster.name": {"test-cluster"},
				"kafka.topic.name":   {topic},
			},
		},
		Config:      map[string]any{"partitions": partition},
		ExecutionId: uuid.New(),
	})
}

func el_leaderOf(t *testing.T, c *kfake.Cluster, topic string, partition int32) int32 {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(c.ListenAddrs()...))
	require.NoError(t, err)
	defer cl.Close()
	req := kmsg.NewPtrMetadataRequest()
	rt := kmsg.NewMetadataRequestTopic()
	rt.Topic = kmsg.StringPtr(topic)
	req.Topics = append(req.Topics, rt)
	resp, err := req.RequestWith(context.Background(), cl)
	require.NoError(t, err)
	for _, p := range resp.Topics[0].Partitions {
		if p.Partition == partition {
			return p.Leader
		}
	}
	t.Fatalf("partition %d not found in topic %s", partition, topic)
	return -1
}

var el_adminKeys = map[int16]int16{
	int16(kmsg.ElectLeaders):              2,
	int16(kmsg.AlterPartitionAssignments): 0,
}

func TestElectNewLeader_Describe(t *testing.T) {
	action := NewKafkaBrokerElectNewLeaderAttack()

	desc := action.Describe()

	assert.Equal(t, fmt.Sprintf("%s.elect-new-leader", kafkaTopicTargetId), desc.Id)
	assert.Equal(t, "Elect New Partition Leader", desc.Label)
	assert.Equal(t, action_kit_api.TimeControlInstantaneous, desc.TimeControl)
	assert.Equal(t, action_kit_api.Attack, desc.Kind)
	require.NotNil(t, desc.TargetSelection)
	assert.Equal(t, kafkaTopicTargetId, desc.TargetSelection.TargetType)
	require.Len(t, desc.Parameters, 1)
	assert.Equal(t, "partitions", desc.Parameters[0].Name)
	assert.Equal(t, KafkaBrokerAttackState{}, action.NewEmptyState())
}

func TestElectNewLeader_Prepare(t *testing.T) {
	c := el_newCluster(t, "steadybit", 3)
	action := NewKafkaBrokerElectNewLeaderAttack()

	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, el_prepareRequest("steadybit", "2"))

	require.NoError(t, err)
	assert.Equal(t, "steadybit", state.Topic)
	assert.Equal(t, int32(2), state.Partition)
	assert.Equal(t, "test-cluster", state.ClusterName)
	assert.Equal(t, c.ListenAddrs(), state.BrokerHosts)
}

func TestElectNewLeader_Prepare_unknown_cluster(t *testing.T) {
	config.SetClustersForTest(map[string]*config.ClusterConfig{})
	action := NewKafkaBrokerElectNewLeaderAttack()

	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, el_prepareRequest("steadybit", "0"))

	require.ErrorContains(t, err, "failed to get cluster config")
}

func TestElectNewLeader_Start_reassigns_and_elects(t *testing.T) {
	c := el_newCluster(t, "steadybit", 3)
	el_advertiseKeys(t, c, el_adminKeys)
	fake := el_installElectFake(c, nil)
	leader := el_leaderOf(t, c, "steadybit", 1)

	action := NewKafkaBrokerElectNewLeaderAttack()
	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, el_prepareRequest("steadybit", 1))
	require.NoError(t, err)

	result, err := action.Start(context.Background(), &state)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	require.NotNil(t, result.Messages)
	require.Len(t, *result.Messages, 1)
	assert.Equal(t, "Successfully elected leader for topic 'steadybit', partition 1.", (*result.Messages)[0].Message)
	assert.Equal(t, action_kit_api.Info, *(*result.Messages)[0].Level)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, 1, fake.elections)
	replicas := fake.reassigned["steadybit"][1]
	require.NotEmpty(t, replicas)
	assert.Equal(t, leader, replicas[len(replicas)-1], "the current leader must be moved to the end of the replica preferences")
}

func TestElectNewLeader_Start_reports_failed_election(t *testing.T) {
	c := el_newCluster(t, "steadybit", 1)
	el_advertiseKeys(t, c, el_adminKeys)
	el_installElectFake(c, kerr.PreferredLeaderNotAvailable)

	action := NewKafkaBrokerElectNewLeaderAttack()
	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, el_prepareRequest("steadybit", 0))
	require.NoError(t, err)

	result, err := action.Start(context.Background(), &state)

	require.NoError(t, err, "a failed election is reported in the result, not as an error")
	require.NotNil(t, result.Error)
	assert.Equal(t, "Election failed for partition(s)", result.Error.Title)
	require.NotNil(t, result.Error.Detail)
	assert.Contains(t, *result.Error.Detail, "partition 0")
	require.Len(t, *result.Messages, 1)
	assert.Equal(t, action_kit_api.Warn, *(*result.Messages)[0].Level)
	assert.Contains(t, (*result.Messages)[0].Message, kerr.PreferredLeaderNotAvailable.Message)
}

func TestElectNewLeader_Start_fails_when_reassignment_is_not_supported(t *testing.T) {
	el_newCluster(t, "steadybit", 1)

	action := NewKafkaBrokerElectNewLeaderAttack()
	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, el_prepareRequest("steadybit", 0))
	require.NoError(t, err)

	result, err := action.Start(context.Background(), &state)

	require.Error(t, err, "kfake does not implement AlterPartitionAssignments, so the action must fail")
	assert.Nil(t, result)
}

func TestElectNewLeader_Start_unknown_cluster(t *testing.T) {
	config.SetClustersForTest(map[string]*config.ClusterConfig{})
	action := NewKafkaBrokerElectNewLeaderAttack()
	state := KafkaBrokerAttackState{ClusterName: "missing", Topic: "steadybit"}

	_, err := action.Start(context.Background(), &state)

	require.ErrorContains(t, err, "failed to get cluster config")
}

func TestRelegateLeader(t *testing.T) {
	tests := []struct {
		name     string
		replicas []int32
		isr      []int32
		leader   int32
		want     []int32
	}{
		{name: "in-sync replicas first, leader last", replicas: []int32{1, 2, 3}, isr: []int32{1, 2, 3}, leader: 1, want: []int32{2, 3, 1}},
		{name: "out-of-sync replicas after in-sync ones", replicas: []int32{1, 2, 3}, isr: []int32{1, 3}, leader: 1, want: []int32{3, 2, 1}},
		{name: "single replica stays leader", replicas: []int32{1}, isr: []int32{1}, leader: 1, want: []int32{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relegateLeader(tt.replicas, tt.isr, tt.leader))
		})
	}
}

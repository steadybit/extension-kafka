// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
)

const cp_cluster = "cp-cluster"

// cp_newCluster starts an in-memory Kafka with three brokers and a "steadybit" topic, and registers it
// as cp_cluster.
func cp_newCluster(t *testing.T) *kfake.Cluster {
	t.Helper()
	c, err := kfake.NewCluster(kfake.SeedTopics(3, "steadybit"), kfake.NumBrokers(3))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		cp_cluster: {SeedBrokers: strings.Join(c.ListenAddrs(), ",")},
	})
	return c
}

func cp_request(topic string, cfg map[string]any) action_kit_api.PrepareActionRequestBody {
	attributes := map[string][]string{"kafka.cluster.name": {cp_cluster}}
	if topic != "" {
		attributes["kafka.topic.name"] = []string{topic}
	}
	return extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target:      &action_kit_api.Target{Attributes: attributes},
		Config:      cfg,
		ExecutionId: uuid.New(),
	})
}

// cp_prepared returns a state prepared for the "steadybit" topic, with a first status call done so the
// previous partition metadata is known.
func cp_prepared(t *testing.T, cfg map[string]any) *PartitionsCheckState {
	t.Helper()
	action := PartitionsCheckAction{}
	state := action.NewEmptyState()
	_, err := action.Prepare(t.Context(), &state, cp_request("steadybit", cfg))
	require.NoError(t, err)
	result, err := action.Start(t.Context(), &state)
	require.NoError(t, err)
	require.Nil(t, result.Error)
	require.Len(t, state.PreviousLeader, 3, "the first poll records every partition")
	return &state
}

func cp_metric(t *testing.T, result *action_kit_api.StatusResult) map[string]string {
	t.Helper()
	require.NotNil(t, result.Metrics)
	require.Len(t, *result.Metrics, 1)
	return (*result.Metrics)[0].Metric
}

func TestCheckPartitions_Describe(t *testing.T) {
	action := NewPartitionsCheckAction()
	description := action.Describe()

	assert.Equal(t, fmt.Sprintf("%s.check-partitions", kafkaTopicTargetId), description.Id)
	assert.Equal(t, "Check Partitions", description.Label)
	assert.Equal(t, action_kit_api.Check, description.Kind)
	assert.Equal(t, action_kit_api.TimeControlInternal, description.TimeControl)
	require.NotNil(t, description.TargetSelection)
	assert.Equal(t, kafkaTopicTargetId, description.TargetSelection.TargetType)
	var names []string
	for _, p := range description.Parameters {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"duration", "expectedChanges", "changeCheckMode", "failEarly"}, names)
	require.NotNil(t, description.Status)
	assert.Equal(t, "2s", *description.Status.CallInterval)
	assert.Equal(t, PartitionsCheckState{}, action.NewEmptyState())
}

func TestCheckPartitions_Prepare(t *testing.T) {
	c := cp_newCluster(t)
	action := PartitionsCheckAction{}

	t.Run("fills the state from the config", func(t *testing.T) {
		state := PartitionsCheckState{}
		before := time.Now()
		_, err := action.Prepare(t.Context(), &state, cp_request("steadybit", map[string]any{
			"duration":        10000,
			"expectedChanges": []string{LeaderChanged},
			"changeCheckMode": stateCheckModeAtLeastOnce,
		}))

		require.NoError(t, err)
		assert.Equal(t, "steadybit", state.TopicName)
		assert.Equal(t, cp_cluster, state.ClusterName)
		assert.Equal(t, c.ListenAddrs(), state.BrokerHosts)
		assert.Equal(t, []string{LeaderChanged}, state.ExpectedChanges)
		assert.Equal(t, stateCheckModeAtLeastOnce, state.StateCheckMode)
		assert.True(t, state.FailEarly, "fail early is the default")
		assert.WithinDuration(t, before.Add(10*time.Second), state.End, 2*time.Second)
		assert.NotNil(t, state.PreviousLeader)
		assert.NotNil(t, state.PreviousReplicas)
		assert.NotNil(t, state.PreviousInSyncReplicas)
		assert.NotNil(t, state.PreviousOfflineReplicas)
	})

	t.Run("fail early can be disabled", func(t *testing.T) {
		state := PartitionsCheckState{}
		_, err := action.Prepare(t.Context(), &state, cp_request("steadybit", map[string]any{
			"duration":  1000,
			"failEarly": false,
		}))

		require.NoError(t, err)
		assert.False(t, state.FailEarly)
		assert.Empty(t, state.ExpectedChanges)
		assert.Empty(t, state.StateCheckMode)
	})

	t.Run("rejects a target without topic name", func(t *testing.T) {
		state := PartitionsCheckState{}
		_, err := action.Prepare(t.Context(), &state, cp_request("", map[string]any{"duration": 1000}))

		assert.EqualError(t, err, "the target is missing the kafka.topic.name attribute")
	})

	t.Run("rejects an unknown cluster", func(t *testing.T) {
		state := PartitionsCheckState{}
		request := cp_request("steadybit", map[string]any{"duration": 1000})
		request.Target.Attributes["kafka.cluster.name"] = []string{"unknown-cluster"}

		_, err := action.Prepare(t.Context(), &state, request)

		assert.ErrorContains(t, err, "failed to get cluster config")
	})
}

func TestCheckPartitions_first_poll_records_metadata_without_changes(t *testing.T) {
	cp_newCluster(t)
	state := cp_prepared(t, map[string]any{"duration": 60000})

	assert.Len(t, state.PreviousReplicas, 3)
	assert.Len(t, state.PreviousInSyncReplicas, 3)
	assert.Len(t, state.PreviousOfflineReplicas, 3)
	for partition, replicas := range state.PreviousReplicas {
		assert.Contains(t, replicas, state.PreviousLeader[partition], "partition %d is led by one of its replicas", partition)
	}

	result, err := TopicCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.False(t, result.Completed)
	assert.Nil(t, result.Error)
	metric := cp_metric(t, result)
	assert.Equal(t, "info", metric["state"])
	assert.Equal(t, "No changes", metric["tooltip"])
	assert.Equal(t, "steadybit", metric["kafka.topic.name"])
}

func TestCheckPartitions_detects_each_kind_of_change(t *testing.T) {
	cp_newCluster(t)
	state := cp_prepared(t, map[string]any{"duration": 60000})
	actualLeader := state.PreviousLeader[0]
	actualReplicas := state.PreviousReplicas[1]
	actualISR := state.PreviousInSyncReplicas[2]

	// Pretend the previous poll saw different metadata than the cluster reports now.
	state.PreviousLeader[0] = 999
	state.PreviousReplicas[1] = []int32{997, 998}
	state.PreviousInSyncReplicas[2] = []int32{996}
	state.PreviousOfflineReplicas[0] = []int32{995}

	result, err := (&PartitionsCheckAction{}).Status(t.Context(), state)

	require.NoError(t, err)
	assert.Nil(t, result.Error, "without expected changes nothing fails")
	tooltip := cp_metric(t, result)["tooltip"]
	assert.True(t, strings.HasPrefix(tooltip, "PARTITION ACTIVITY"))
	for _, change := range []string{LeaderChanged, ReplicasChanged, InSyncReplicasChanged, OfflineReplicasChanged} {
		assert.Contains(t, tooltip, change)
	}
	assert.Contains(t, tooltip, "previous: 999")
	assert.Equal(t, "danger", cp_metric(t, result)["state"])

	assert.Equal(t, actualLeader, state.PreviousLeader[0], "the state follows the cluster again")
	assert.Equal(t, actualReplicas, state.PreviousReplicas[1])
	assert.Equal(t, actualISR, state.PreviousInSyncReplicas[2])
	assert.Empty(t, state.PreviousOfflineReplicas[0])

	result, err = TopicCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Equal(t, "No changes", cp_metric(t, result)["tooltip"], "a change is only reported once")
}

func TestCheckPartitions_all_the_time_accepts_the_expected_change(t *testing.T) {
	cp_newCluster(t)
	state := cp_prepared(t, map[string]any{
		"duration":        60000,
		"expectedChanges": []string{LeaderChanged},
		"changeCheckMode": stateCheckModeAllTheTime,
	})
	state.PreviousLeader[0] = 999

	result, err := TopicCheckStatus(t.Context(), state)

	require.NoError(t, err)
	assert.Nil(t, result.Error)
	assert.Equal(t, "success", cp_metric(t, result)["state"])
}

func TestCheckPartitions_all_the_time_fails_early_on_an_unexpected_change(t *testing.T) {
	cp_newCluster(t)
	state := cp_prepared(t, map[string]any{
		"duration":        60000,
		"expectedChanges": []string{ReplicasChanged},
		"changeCheckMode": stateCheckModeAllTheTime,
	})
	state.PreviousLeader[0] = 999

	result, err := TopicCheckStatus(t.Context(), state)

	require.NoError(t, err)
	require.NotNil(t, result.Error)
	assert.Equal(t, action_kit_api.Failed, *result.Error.Status)
	assert.Contains(t, result.Error.Title, "Topic 'steadybit' has an unexpected change 'leader changed'")
	assert.False(t, result.Completed)
}

func TestCheckPartitions_all_the_time_fails_at_the_end_with_the_first_deviation(t *testing.T) {
	cp_newCluster(t)
	state := cp_prepared(t, map[string]any{
		"duration":        60000,
		"expectedChanges": []string{ReplicasChanged},
		"changeCheckMode": stateCheckModeAllTheTime,
		"failEarly":       false,
	})

	state.PreviousLeader[0] = 999
	result, err := TopicCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Nil(t, result.Error, "the step is not over yet")
	assert.True(t, state.DeviationSeen)
	firstTitle := state.DeviationTitle
	assert.Contains(t, firstTitle, "had an unexpected change 'leader changed'")

	state.PreviousInSyncReplicas[1] = []int32{996}
	_, err = TopicCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Equal(t, firstTitle, state.DeviationTitle, "a later deviation does not overwrite the first one")

	state.End = time.Now().Add(-time.Second)
	result, err = TopicCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.True(t, result.Completed)
	require.NotNil(t, result.Error, "the deviation is reported once the step ends, even though the topic recovered")
	assert.Equal(t, firstTitle, result.Error.Title)
}

func TestCheckPartitions_at_least_once(t *testing.T) {
	cp_newCluster(t)

	t.Run("succeeds when the expected change was seen once", func(t *testing.T) {
		state := cp_prepared(t, map[string]any{
			"duration":        60000,
			"expectedChanges": []string{InSyncReplicasChanged},
			"changeCheckMode": stateCheckModeAtLeastOnce,
		})
		state.PreviousInSyncReplicas[0] = []int32{996}

		result, err := TopicCheckStatus(t.Context(), state)
		require.NoError(t, err)
		assert.Nil(t, result.Error)
		assert.True(t, state.StateCheckSuccess)

		state.End = time.Now().Add(-time.Second)
		result, err = TopicCheckStatus(t.Context(), state)
		require.NoError(t, err)
		assert.True(t, result.Completed)
		assert.Nil(t, result.Error)
	})

	t.Run("fails at the end when the expected change never happened", func(t *testing.T) {
		state := cp_prepared(t, map[string]any{
			"duration":        60000,
			"expectedChanges": []string{InSyncReplicasChanged},
			"changeCheckMode": stateCheckModeAtLeastOnce,
		})
		state.PreviousLeader[0] = 999 // a change, but not the expected one

		result, err := TopicCheckStatus(t.Context(), state)
		require.NoError(t, err)
		assert.Nil(t, result.Error, "at least once is only evaluated at the end")

		state.End = time.Now().Add(-time.Second)
		result, err = TopicCheckStatus(t.Context(), state)
		require.NoError(t, err)
		assert.True(t, result.Completed)
		require.NotNil(t, result.Error)
		assert.Equal(t, "Topic 'steadybit' didn't have the expected changes '[in sync replicas changed]' at least once.", result.Error.Title)
	})
}

func TestCheckPartitions_unknown_topic_reports_no_changes(t *testing.T) {
	cp_newCluster(t)
	state := &PartitionsCheckState{
		TopicName:               "does-not-exist",
		ClusterName:             cp_cluster,
		End:                     time.Now().Add(time.Minute),
		PreviousLeader:          map[int32]int32{},
		PreviousReplicas:        map[int32][]int32{},
		PreviousInSyncReplicas:  map[int32][]int32{},
		PreviousOfflineReplicas: map[int32][]int32{},
	}
	cfg, err := config.GetClusterConfig(cp_cluster)
	require.NoError(t, err)
	state.BrokerHosts = strings.Split(cfg.SeedBrokers, ",")

	result, err := TopicCheckStatus(t.Context(), state)

	require.NoError(t, err)
	assert.Empty(t, state.PreviousLeader)
	assert.Equal(t, "info", cp_metric(t, result)["state"])
}

func TestCheckPartitions_status_errors(t *testing.T) {
	t.Run("unknown cluster", func(t *testing.T) {
		cp_newCluster(t)
		state := &PartitionsCheckState{TopicName: "steadybit", ClusterName: "unknown-cluster"}

		result, err := TopicCheckStatus(t.Context(), state)

		assert.Nil(t, result)
		assert.ErrorContains(t, err, "failed to get cluster config")
	})

	t.Run("unreachable cluster", func(t *testing.T) {
		c := cp_newCluster(t)
		state := cp_prepared(t, map[string]any{"duration": 60000})
		c.Close()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()

		result, err := (&PartitionsCheckAction{}).Start(ctx, state)

		assert.Nil(t, result)
		assert.ErrorContains(t, err, "Failed to retrieve topics from Kafka for name steadybit")
	})
}

func TestToTopicChangeMetric(t *testing.T) {
	now := time.Now()

	metric := toTopicChangeMetric("orders", nil, nil, map[string][]string{}, now)
	assert.Equal(t, "kafka_consumer_group_state", *metric.Name)
	assert.Equal(t, now, metric.Timestamp)
	assert.Equal(t, map[string]string{"kafka.topic.name": "orders", "url": "", "state": "info", "tooltip": "No changes"}, metric.Metric)

	changes := map[string][]string{LeaderChanged: {"previous: 1,actual: 2"}}
	metric = toTopicChangeMetric("orders", []string{LeaderChanged}, []string{LeaderChanged}, changes, now)
	assert.Equal(t, "success", metric.Metric["state"])
	assert.Equal(t, "PARTITION ACTIVITY\nleader changed:\nprevious: 1,actual: 2", metric.Metric["tooltip"])

	metric = toTopicChangeMetric("orders", []string{ReplicasChanged}, []string{LeaderChanged}, changes, now)
	assert.Equal(t, "danger", metric.Metric["state"])
}

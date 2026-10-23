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
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

const dl_topic = "dl-topic"

// dl_newCluster starts a cluster with a two-partition topic, registers it as "dl-cluster", and produces
// perPartition records to each partition. It returns the seed brokers.
func dl_newCluster(t *testing.T, perPartition int) []string {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, dl_topic))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	brokers := c.ListenAddrs()

	saved := config.Config.Clusters
	t.Cleanup(func() { config.SetClustersForTest(saved) })
	config.SetClustersForTest(map[string]*config.ClusterConfig{"dl-cluster": {SeedBrokers: strings.Join(brokers, ",")}})

	producer, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.RecordPartitioner(kgo.ManualPartitioner()))
	require.NoError(t, err)
	defer producer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for partition := int32(0); partition < 2; partition++ {
		for i := range perPartition {
			r := &kgo.Record{Topic: dl_topic, Partition: partition, Value: fmt.Appendf(nil, "record-%d", i)}
			require.NoError(t, producer.ProduceSync(ctx, r).FirstErr())
		}
	}
	return brokers
}

// dl_startOffsets returns the first offset still readable in each partition of the test topic.
func dl_startOffsets(t *testing.T, brokers []string) map[int32]int64 {
	t.Helper()
	admin, err := createNewAdminClientWithConfig(brokers, &config.ClusterConfig{})
	require.NoError(t, err)
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	offsets, err := admin.ListStartOffsets(ctx, dl_topic)
	require.NoError(t, err)
	require.NoError(t, offsets.Error())
	result := map[int32]int64{}
	offsets.Each(func(o kadm.ListedOffset) { result[o.Partition] = o.Offset })
	return result
}

func dl_state(brokers []string, partitions []string, offset int64) *DeleteRecordsState {
	return &DeleteRecordsState{TopicName: dl_topic, Partitions: partitions, Offset: offset, BrokerHosts: brokers, ClusterName: "dl-cluster"}
}

func TestDeleteRecords_Describe(t *testing.T) {
	action := NewDeleteRecordsAttack()
	desc := action.Describe()

	assert.Equal(t, kafkaTopicTargetId+".delete-records", desc.Id)
	assert.Equal(t, "Trigger Delete Records", desc.Label)
	assert.Equal(t, action_kit_api.TimeControlInstantaneous, desc.TimeControl)
	assert.Equal(t, action_kit_api.Attack, desc.Kind)
	require.NotNil(t, desc.TargetSelection)
	assert.Equal(t, kafkaTopicTargetId, desc.TargetSelection.TargetType)
	require.Len(t, desc.Parameters, 2)
	assert.Equal(t, "partitions", desc.Parameters[0].Name)
	assert.Equal(t, "offset", desc.Parameters[1].Name)
	assert.Equal(t, DeleteRecordsState{}, action.NewEmptyState())
}

func TestDeleteRecords_Prepare(t *testing.T) {
	saved := config.Config.Clusters
	t.Cleanup(func() { config.SetClustersForTest(saved) })
	config.SetClustersForTest(map[string]*config.ClusterConfig{"dl-cluster": {SeedBrokers: "broker-a:9092,broker-b:9092"}})
	action := &DeleteRecordsAttack{}

	request := func(cluster string) action_kit_api.PrepareActionRequestBody {
		return extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
			Target: &action_kit_api.Target{Attributes: map[string][]string{
				"kafka.cluster.name": {cluster},
				"kafka.topic.name":   {dl_topic},
			}},
			Config:      map[string]any{"partitions": []string{"0", "1"}, "offset": 3},
			ExecutionId: uuid.New(),
		})
	}

	state := action.NewEmptyState()
	result, err := action.Prepare(context.Background(), &state, request("dl-cluster"))
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, DeleteRecordsState{
		TopicName:   dl_topic,
		Partitions:  []string{"0", "1"},
		Offset:      3,
		BrokerHosts: []string{"broker-a:9092", "broker-b:9092"},
		ClusterName: "dl-cluster",
	}, state)

	state = action.NewEmptyState()
	_, err = action.Prepare(context.Background(), &state, request("dl-unknown"))
	assert.ErrorContains(t, err, "failed to get cluster config")
}

func TestDeleteRecords_Start_deletes_all_but_the_newest_records(t *testing.T) {
	brokers := dl_newCluster(t, 10)

	result, err := (&DeleteRecordsAttack{}).Start(context.Background(), dl_state(brokers, []string{"0"}, 3))
	require.NoError(t, err)

	assert.Equal(t, map[int32]int64{0: 7, 1: 0}, dl_startOffsets(t, brokers), "partition 0 keeps its 3 newest records, partition 1 is untouched")
	require.NotNil(t, result)
	require.Len(t, *result.Messages, 1)
	assert.Equal(t, fmt.Sprintf("Trigger delete records for topic %s for partition 0, moving offset at 7", dl_topic), (*result.Messages)[0].Message)
}

func TestDeleteRecords_Start_deletes_everything_with_offset_zero_and_never_goes_below_zero(t *testing.T) {
	brokers := dl_newCluster(t, 5)

	result, err := (&DeleteRecordsAttack{}).Start(context.Background(), dl_state(brokers, []string{"0", "1"}, 0))
	require.NoError(t, err)
	assert.Equal(t, map[int32]int64{0: 5, 1: 5}, dl_startOffsets(t, brokers))
	assert.Len(t, strings.Split((*result.Messages)[0].Message, "\n"), 2, "one line per partition")

	// Keeping more records than a partition holds deletes nothing.
	brokers = dl_newCluster(t, 5)
	_, err = (&DeleteRecordsAttack{}).Start(context.Background(), dl_state(brokers, []string{"1"}, 100))
	require.NoError(t, err)
	assert.Equal(t, map[int32]int64{0: 0, 1: 0}, dl_startOffsets(t, brokers))
}

func TestDeleteRecords_Start_errors(t *testing.T) {
	brokers := dl_newCluster(t, 1)
	action := &DeleteRecordsAttack{}
	ctx := context.Background()

	_, err := action.Start(ctx, dl_state(brokers, []string{"not-a-number"}, 0))
	assert.ErrorContains(t, err, "Failed to convert partition not-a-number")

	_, err = action.Start(ctx, dl_state(brokers, []string{"7"}, 0))
	assert.ErrorContains(t, err, "Failed to find offset for topic dl-topic and partition 7")

	unknownTopic := dl_state(brokers, []string{"0"}, 0)
	unknownTopic.TopicName = "dl-missing"
	_, err = action.Start(ctx, unknownTopic)
	assert.Error(t, err)

	unknownCluster := dl_state(brokers, []string{"0"}, 0)
	unknownCluster.ClusterName = "dl-unknown"
	_, err = action.Start(ctx, unknownCluster)
	assert.ErrorContains(t, err, "failed to get cluster config")

	config.SetClustersForTest(map[string]*config.ClusterConfig{"dl-cluster": {SeedBrokers: strings.Join(brokers, ","), KafkaClusterCertKeyFile: "/only/the/key"}})
	_, err = action.Start(ctx, dl_state(brokers, []string{"0"}, 0))
	assert.ErrorContains(t, err, "mutual TLS requires both")
}

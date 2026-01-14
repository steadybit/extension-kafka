// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

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
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestCheckTopicLag_Describe(t *testing.T) {
	//Given
	action := ConsumerGroupLagCheckAction{}

	//When
	response := action.Describe()

	//Then
	assert.Equal(t, "Check the consumer lag for a given topic (lag is calculated by the difference between topic offset and consumer offset)", response.Description)
	assert.Equal(t, "Check Topic Lag", response.Label)
	assert.Equal(t, kafkaConsumerTargetId, response.TargetSelection.TargetType)
	assert.Equal(t, fmt.Sprintf("%s.check-lag", kafkaConsumerTargetId), response.Id)
	assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
}

func TestCheckConsumerGroupLag_Prepare(t *testing.T) {
	// Initialize cluster configuration for test
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {
			SeedBrokers: "localhost:9092",
		},
	})

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupLagCheckState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
						"kafka.cluster.name":        {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"duration":      10000,
					"topic":         "steadybit",
					"acceptableLag": "1",
				},
				ExecutionId: uuid.New(),
			}),
			wantedState: &ConsumerGroupLagCheckState{
				ConsumerGroupName: "steadybit",
				StateCheckSuccess: true,
				Topic:             "steadybit",
				AcceptableLag:     1,
			},
		},
		{
			name: "Should return error for consumer group name",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{},
				},
				Config: map[string]interface{}{
					"duration":      10000,
					"topic":         "steadybit",
					"acceptableLag": "1",
				},
				ExecutionId: uuid.New(),
			}),
			wantedError: extension_kit.ToError("the target is missing the kafka.consumer-group.name attribute", nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupLagCheckAction{}
			state := ConsumerGroupLagCheckState{}
			request := tt.requestBody

			//When
			_, err := action.Prepare(t.Context(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedState.AcceptableLag, state.AcceptableLag)
				assert.Equal(t, state.ConsumerGroupName, state.ConsumerGroupName)
				assert.Equal(t, state.Topic, state.Topic)
				assert.False(t, state.StateCheckSuccess)
				assert.NotNil(t, state.End)
			}
		})
	}
}

func TestCheckConsumerGroupLag_Status(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	require.NoError(t, err)
	defer c.Close()

	seeds := c.ListenAddrs()
	seedBrokers := strings.Join(seeds, ",")

	// Initialize cluster configuration for test
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {
			SeedBrokers: seedBrokers,
		},
	})
	// One client can both produce and consume!
	// Consuming can either be direct (no consumer group), or through a group. Below, we use a group.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumerGroup("steadybit"),
		kgo.DefaultProduceTopic("steadybit"),
		kgo.ConsumeTopics("steadybit"),
	)
	require.NoError(t, err)
	defer cl.Close()

	// produce messages for lags
	for i := 0; i < 10; i++ {
		cl.ProduceSync(context.TODO(), &kgo.Record{Key: []byte("steadybit"), Value: []byte("test")})
	}

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupLagCheckState
	}{
		{
			name: "Should return status ok",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
						"kafka.cluster.name":        {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"duration":      5000,
					"topic":         "steadybit",
					"acceptableLag": "15",
				},
				ExecutionId: uuid.New(),
			}),
			wantedState: &ConsumerGroupLagCheckState{
				ConsumerGroupName: "steadybit",
				AcceptableLag:     int64(15),
				StateCheckSuccess: true,
				Topic:             "steadybit",
			},
		},
		{
			name: "Should return status ko",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
						"kafka.cluster.name":        {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"duration":      5000,
					"topic":         "steadybit",
					"acceptableLag": "1",
				},
				ExecutionId: uuid.New(),
			}),
			wantedState: &ConsumerGroupLagCheckState{
				ConsumerGroupName: "steadybit",
				AcceptableLag:     int64(1),
				StateCheckSuccess: true,
				Topic:             "steadybit",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupLagCheckAction{}
			state := ConsumerGroupLagCheckState{}
			request := tt.requestBody

			//When
			_, errPrepare := action.Prepare(t.Context(), &state, request)
			statusResult, errStatus := action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.AcceptableLag, state.AcceptableLag)
				assert.Equal(t, tt.wantedState.Topic, state.Topic)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.False(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}

			time.Sleep(6 * time.Second)

			// Completed
			_, errStatus = action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.AcceptableLag, state.AcceptableLag)
				assert.Equal(t, tt.wantedState.Topic, state.Topic)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.NotNil(t, state.End)
			}
		})
	}
}

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
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestCheckBrokers_Describe(t *testing.T) {
	//Given
	action := CheckBrokersAction{}

	//When
	response := action.Describe()

	//Then
	assert.Equal(t, "Check activity of brokers.", response.Description)
	assert.Equal(t, "Check Brokers", response.Label)
	assert.Equal(t, fmt.Sprintf("%s.check", kafkaBrokerTargetId), response.Id)
	assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
	require.NotNil(t, response.TargetSelection)
	assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
}

func TestCheckBrokers_Prepare(t *testing.T) {
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

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"expectedChanges": []string{"test"},
					"changeCheckMode": "allTheTime",
					"duration":        10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &CheckBrokersState{
				ExpectedChanges:   []string{"test"},
				StateCheckMode:    "allTheTime",
				StateCheckSuccess: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := CheckBrokersAction{}
			state := CheckBrokersState{}
			request := tt.requestBody
			//When
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedState.ExpectedChanges, state.ExpectedChanges)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.StateCheckSuccess, state.StateCheckSuccess)
				assert.NotNil(t, state.End)
			}
		})
	}
}

func TestCheckBrokers_Status(t *testing.T) {
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
		kgo.ConsumeTopics("steadybit"),
	)
	require.NoError(t, err)
	defer cl.Close()

	tests := []struct {
		name        string
		killNode    *int
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name:     "Should return status ok",
			killNode: extutil.Ptr(1),
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"expectedChanges": []string{"kafka broker with downtime"},
					"changeCheckMode": "atLeastOnce",
					"duration":        5000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &CheckBrokersState{
				StateCheckMode:    "atLeastOnce",
				StateCheckSuccess: true,
			},
		},
		{
			name:     "Should return status ok with all the time check mode",
			killNode: extutil.Ptr(2),
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]interface{}{
					"expectedChanges": []string{"Dead"},
					"changeCheckMode": "allTheTime",
					"duration":        5000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &CheckBrokersState{
				ExpectedChanges:   []string{"kafka broker with downtime"},
				StateCheckMode:    "allTheTime",
				StateCheckSuccess: false,
				BrokerNodes:       []int32{1, 2, 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := CheckBrokersAction{}
			state := CheckBrokersState{}
			request := tt.requestBody
			//When
			_, errPrepare := action.Prepare(t.Context(), &state, request)
			statusResult, errStatus := action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.False(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}

			if tt.wantedError != nil {
				err := c.RemoveNode(int32(*tt.killNode))
				require.NoError(t, err)
			}
			time.Sleep(6 * time.Second)

			// Completed
			statusResult, errStatus = action.Status(t.Context(), &state)
			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.True(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}
		})
	}
}

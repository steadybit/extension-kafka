// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"strings"
	"testing"
	"time"
)

func TestCheckConsumerGroup_Describe(t *testing.T) {
	//Given
	action := ConsumerGroupCheckAction{}

	//When
	response := action.Describe()

	//Then
	assert.Equal(t, "Check the consumer state", response.Description)
	assert.Equal(t, "Check Consumer State", response.Label)
	assert.Equal(t, kafkaConsumerTargetId, response.TargetSelection.TargetType)
	assert.Equal(t, fmt.Sprintf("%s.check", kafkaConsumerTargetId), response.Id)
	assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
}

func TestCheckConsumerGroup_Prepare(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupCheckState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"expectedStateList": []string{"test"},
					"stateCheckMode":    "test",
					"duration":          10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &ConsumerGroupCheckState{
				ConsumerGroupName: "steadybit",
				ExpectedState:     []string{"test"},
				StateCheckMode:    "test",
				StateCheckSuccess: true,
				TopicName:         "steadybit",
			},
		},
		{
			name: "Should return error for consumer group name",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{},
				},
				Config: map[string]interface{}{
					"expectedStateList": []string{"test"},
					"stateCheckMode":    "test",
					"duration":          10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedError: extension_kit.ToError("the target is missing the kafka.consumer-group.name attribute", nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupCheckAction{}
			state := ConsumerGroupCheckState{}
			request := tt.requestBody

			//When
			_, err := action.Prepare(t.Context(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.Equal(t, tt.wantedState.ExpectedState, state.ExpectedState)
				assert.False(t, state.StateCheckSuccess)
				assert.NotNil(t, state.End)
			}
		})
	}
}

func TestCheckConsumerGroup_Status(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	require.NoError(t, err)
	defer c.Close()

	seeds := c.ListenAddrs()
	config.Config.SeedBrokers = strings.Join(seeds, ",")
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
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupCheckState
	}{
		{
			name: "Should return status ok",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"expectedStateList": []string{"Dead"},
					"stateCheckMode":    "atLeastOnce",
					"duration":          5000,
				},
				ExecutionId: uuid.New(),
			}),
			wantedState: &ConsumerGroupCheckState{
				ConsumerGroupName: "steadybit",
				ExpectedState:     []string{"Dead"},
				StateCheckMode:    "atLeastOnce",
				StateCheckSuccess: true,
				TopicName:         "steadybit",
			},
		},
		{
			name: "Should return status ok with all the time check mode",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"expectedStateList": []string{"Dead"},
					"stateCheckMode":    "allTheTime",
					"duration":          5000,
				},
				ExecutionId: uuid.New(),
			}),
			wantedState: &ConsumerGroupCheckState{
				ConsumerGroupName: "steadybit",
				ExpectedState:     []string{"Dead"},
				StateCheckMode:    "allTheTime",
				StateCheckSuccess: false,
				TopicName:         "steadybit",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupCheckAction{}
			state := ConsumerGroupCheckState{}
			request := tt.requestBody

			//When
			_, errPrepare := action.Prepare(t.Context(), &state, request)
			statusResult, errStatus := action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.False(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}

			time.Sleep(6 * time.Second)

			// Completed
			statusResult, errStatus = action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.True(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}
		})
	}
}

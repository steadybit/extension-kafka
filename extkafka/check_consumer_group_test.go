// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"strings"
	"testing"
	"time"
)

func TestCheckConsumerGroup_Describe(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupCheckState
	}{
		{
			name: "Should return description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		})
	}
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
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, "test", tt.wantedState.StateCheckMode)
				assert.Equal(t, "steadybit", state.ConsumerGroupName)
				assert.Equal(t, []string{"test"}, state.ExpectedState)
				assert.Equal(t, false, state.StateCheckSuccess)
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
	if err != nil {
		panic(err)
	}
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
	if err != nil {
		panic(err)
	}
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
			_, errPrepare := action.Prepare(context.TODO(), &state, request)
			statusResult, errStatus := action.Status(context.TODO(), &state)
			time.Sleep(6 * time.Second)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.Equal(t, false, statusResult.Completed)
				assert.NotNil(t, state.End)
			}

			// Completed
			statusResult, errStatus = action.Status(context.TODO(), &state)
			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.ConsumerGroupName, state.ConsumerGroupName)
				assert.Equal(t, true, statusResult.Completed)
				assert.NotNil(t, state.End)
			}
		})
	}
}

//func TestAction_Stop(t *testing.T) {
//
//	tests := []struct {
//		name             string
//		requestBody      action_kit_api.StopActionRequestBody
//		state            *KafkaBrokerAttackState
//		executionRunData *ExecutionRunData
//		wantedError      error
//	}{
//		{
//			name:        "Should successfully stop the action",
//			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
//			state: &KafkaBrokerAttackState{
//				ExecutionID: uuid.New(),
//				SuccessRate: 40,
//			},
//			executionRunData: getExecutionRunData(5, 10),
//			wantedError:      nil,
//		}, {
//			name:        "Should fail because of low success rate",
//			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
//			state: &KafkaBrokerAttackState{
//				ExecutionID: uuid.New(),
//				SuccessRate: 100,
//			},
//			executionRunData: getExecutionRunData(4, 11),
//			wantedError:      extension_kit.ToError("Success Rate (36.36%) was below 100%", nil),
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			//Given
//			saveExecutionRunData(tt.state.ExecutionID, tt.executionRunData)
//			//When
//			result, err := stop(tt.state)
//
//			//Then
//			if tt.wantedError != nil && result.Error == nil {
//				assert.EqualError(t, err, tt.wantedError.Error())
//			} else if tt.wantedError != nil && result.Error != nil {
//				assert.Equal(t, tt.wantedError.Error(), result.Error.Title)
//			} else if tt.wantedError == nil && result.Error != nil {
//				assert.Fail(t, "Should not have error", result.Error.Title)
//			} else {
//				assert.NoError(t, err)
//			}
//		})
//	}
//}
//
//func getExecutionRunData(successCounter uint64, counter uint64) *ExecutionRunData {
//	data := &ExecutionRunData{
//		requestSuccessCounter: atomic.Uint64{},
//		requestCounter:        atomic.Uint64{},
//	}
//	data.requestCounter.Store(counter)
//	data.requestSuccessCounter.Store(successCounter)
//	return data
//
//}

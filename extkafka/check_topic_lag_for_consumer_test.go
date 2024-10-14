/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"testing"
	"time"
)

func TestCheckTopicLag_Describe(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *ConsumerGroupLagCheckState
	}{
		{
			name: "Should return description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupLagCheckAction{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "check if the consumer group have lags on a topic", response.Description)
			assert.Equal(t, "Check Topic Lag", response.Label)
			assert.Equal(t, kafkaConsumerTargetId, response.TargetSelection.TargetType)
			assert.Equal(t, fmt.Sprintf("%s.check-lag", kafkaConsumerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
	}
}

func TestCheckConsumerGroupLag_Prepare(t *testing.T) {
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
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, int64(1), tt.wantedState.AcceptableLag)
				assert.Equal(t, "steadybit", state.ConsumerGroupName)
				assert.Equal(t, "steadybit", state.Topic)
				assert.Equal(t, false, state.StateCheckSuccess)
				assert.NotNil(t, state.End)
			}
		})
	}
}

func TestCheckConsumerGroupLag_Status(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.Ports(9092, 9093, 9094),
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	seeds := []string{"localhost:9092"}
	// One client can both produce and consume!
	// Consuming can either be direct (no consumer group), or through a group. Below, we use a group.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumerGroup("steadybit"),
		kgo.DefaultProduceTopic("steadybit"),
		kgo.ConsumeTopics("steadybit"),
	)
	if err != nil {
		panic(err)
	}
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := ConsumerGroupLagCheckAction{}
			state := ConsumerGroupLagCheckState{}
			request := tt.requestBody
			//When
			_, errPrepare := action.Prepare(context.TODO(), &state, request)
			statusResult, errStatus := action.Status(context.TODO(), &state)
			time.Sleep(6 * time.Second)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.AcceptableLag, state.AcceptableLag)
				assert.Equal(t, tt.wantedState.Topic, state.Topic)
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
				assert.Equal(t, tt.wantedState.AcceptableLag, state.AcceptableLag)
				assert.Equal(t, tt.wantedState.Topic, state.Topic)
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

/*
* Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kfake"
	"testing"
	"time"
)

func TestNewHTTPCheckActionPeriodically_Prepare(t *testing.T) {
	action := produceMessageActionPeriodically{}

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *KafkaBrokerAttackState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.topic.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"recordsPerSecond": 1,
					"maxConcurrent":    4,
					"recordKey":        "steadybit5",
					"recordValue":      "test5",
					"recordHeaders": []any{
						map[string]any{"key": "test", "value": "test"},
					},
					"duration": 10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &KafkaBrokerAttackState{
				ConsumerGroup:            "",
				Topic:                    "steadybit",
				RecordKey:                "steadybit5",
				RecordValue:              "test5",
				MaxConcurrent:            4,
				DelayBetweenRequestsInMS: 1000,
				RecordHeaders:            map[string]string{"test": "test"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			state := action.NewEmptyState()
			request := tt.requestBody
			//When
			_, err := action.Prepare(context.Background(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedState.RecordHeaders, state.RecordHeaders)
				assert.Equal(t, tt.wantedState.MaxConcurrent, state.MaxConcurrent)
				assert.Equal(t, tt.wantedState.DelayBetweenRequestsInMS, state.DelayBetweenRequestsInMS)
				assert.Equal(t, tt.wantedState.SuccessRate, state.SuccessRate)
				assert.NotNil(t, state.ExecutionID)
				assert.NotNil(t, state.Timeout)
			}
		})
	}
}

func TestNewHTTPCheckActionPeriodically_All_Success(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.Ports(9092, 9093, 9094),
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	//prepare the action
	action := produceMessageActionPeriodically{}
	state := action.NewEmptyState()
	prepareActionRequestBody := extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target: &action_kit_api.Target{
			Attributes: map[string][]string{
				"kafka.topic.name": {"steadybit"},
			},
		},
		Config: map[string]interface{}{
			"recordsPerSecond": 1,
			"maxConcurrent":    4,
			"recordKey":        "steadybit5",
			"recordValue":      "test5",
			"recordHeaders": []any{
				map[string]any{"key": "test", "value": "test"},
			},
			"duration": 5000,
		},
		ExecutionId: uuid.New(),
	})

	// Prepare
	prepareResult, err := action.Prepare(context.Background(), &state, prepareActionRequestBody)
	assert.NoError(t, err)
	assert.Nil(t, prepareResult)
	assert.Greater(t, state.DelayBetweenRequestsInMS, extutil.ToInt64(0))

	executionRunData, err := action.getExecutionRunData(state.ExecutionID)
	assert.NoError(t, err)
	assert.NotNil(t, executionRunData)
	// Start
	startResult, err := action.Start(context.Background(), &state)
	assert.NoError(t, err)
	assert.Nil(t, startResult)

	// Status
	statusResult, err := action.Status(context.Background(), &state)
	assert.NoError(t, err)
	assert.NotNil(t, statusResult.Metrics)
	time.Sleep(10 * time.Second)
	// Status completed
	statusResult, err = action.Status(context.Background(), &state)
	assert.NoError(t, err)
	assert.Equal(t, false, statusResult.Completed)

	assert.Equal(t, uint64(10), executionRunData.requestCounter.Load())
	// Stop
	stopResult, err := action.Stop(context.Background(), &state)
	assert.NoError(t, err)
	assert.NotNil(t, stopResult.Metrics)
	assert.Nil(t, stopResult.Error)
	assert.Equal(t, executionRunData.requestSuccessCounter.Load(), uint64(10))
}

func TestNewHTTPCheckActionPeriodically_All_Failure(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.Ports(9092, 9093, 9094),
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	//prepare the action
	action := produceMessageActionPeriodically{}
	state := action.NewEmptyState()
	prepareActionRequestBody := extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target: &action_kit_api.Target{
			Attributes: map[string][]string{
				"kafka.topic.name": {"invalid"},
			},
		},
		Config: map[string]interface{}{
			"recordsPerSecond": 1,
			"maxConcurrent":    4,
			"recordKey":        "steadybit5",
			"recordValue":      "test5",
			"recordHeaders": []any{
				map[string]any{"key": "test", "value": "test"},
			},
			"recordAttributes": "243",
			"duration":         10000,
			"successRate":      100,
		},
		ExecutionId: uuid.New(),
	})

	// Prepare
	prepareResult, err := action.Prepare(context.Background(), &state, prepareActionRequestBody)
	assert.NoError(t, err)
	assert.Nil(t, prepareResult)
	assert.Greater(t, state.DelayBetweenRequestsInMS, extutil.ToInt64(0))

	// Start
	startResult, err := action.Start(context.Background(), &state)
	assert.NoError(t, err)
	assert.Nil(t, startResult)

	// Status
	statusResult, err := action.Status(context.Background(), &state)
	assert.NoError(t, err)
	assert.NotNil(t, statusResult.Metrics)
	time.Sleep(5 * time.Second)
	// Status completed
	statusResult, err = action.Status(context.Background(), &state)
	assert.NoError(t, err)
	assert.Equal(t, statusResult.Completed, false)

	executionRunData, err := action.getExecutionRunData(state.ExecutionID)
	assert.NoError(t, err)
	assert.Greater(t, executionRunData.requestCounter.Load(), uint64(0))
	// Stop
	stopResult, err := action.Stop(context.Background(), &state)
	assert.NoError(t, err)
	assert.NotNil(t, stopResult.Metrics)
	assert.NotNil(t, stopResult.Error)
	assert.Equal(t, stopResult.Error.Title, "Success Rate (0.00%) was below 100%")
	assert.Equal(t, executionRunData.requestSuccessCounter.Load(), uint64(0))
}

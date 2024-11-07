/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"testing"
)

func TestAction_Prepare(t *testing.T) {

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
					"numberOfRecords": 10,
					"maxConcurrent":   4,
					"recordKey":       "steadybit5",
					"recordValue":     "test5",
					"recordHeaders": []any{
						map[string]any{"key": "test", "value": "test"},
					},
					"recordAttributes": "243",
					"duration":         10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &KafkaBrokerAttackState{
				ConsumerGroup:   "",
				Topic:           "steadybit",
				RecordKey:       "steadybit5",
				RecordValue:     "test5",
				MaxConcurrent:   4,
				NumberOfRecords: 10,
				RecordHeaders:   map[string]string{"test": "test"},
			},
		},
		{
			name: "Should return error for recordHeaders",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.topic.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"action":        "prepare",
					"maxConcurrent": 4,
					"recordHeaders": "test:test",
				},
				ExecutionId: uuid.New(),
			}),

			wantedError: extension_kit.ToError("failed to interpret config value for recordHeaders as a key/value array", nil),
		},
		{
			name: "Should return error for maxConcurrent",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.topic.name": {"steadybit"},
					},
				},
				Config: map[string]interface{}{
					"action":        "prepare",
					"maxConcurrent": 0,
				},
				ExecutionId: uuid.New(),
			}),

			wantedError: extension_kit.ToError("max concurrent can't be zero", nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			state := KafkaBrokerAttackState{}
			request := tt.requestBody
			//When
			_, err := prepare(request, &state, func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool { return false })

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedState.RecordHeaders, state.RecordHeaders)
				assert.Equal(t, tt.wantedState.MaxConcurrent, state.MaxConcurrent)
				assert.Equal(t, tt.wantedState.NumberOfRecords, state.NumberOfRecords)
				assert.Equal(t, tt.wantedState.SuccessRate, state.SuccessRate)
				assert.NotNil(t, state.ExecutionID)
				assert.NotNil(t, state.Timeout)
			}
		})
	}
}

func TestAction_Stop(t *testing.T) {

	tests := []struct {
		name             string
		requestBody      action_kit_api.StopActionRequestBody
		state            *KafkaBrokerAttackState
		executionRunData *ExecutionRunData
		wantedError      error
	}{
		{
			name:        "Should successfully stop the action",
			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
			state: &KafkaBrokerAttackState{
				ExecutionID: uuid.New(),
				SuccessRate: 40,
			},
			executionRunData: getExecutionRunData(5, 10),
			wantedError:      nil,
		}, {
			name:        "Should fail because of low success rate",
			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
			state: &KafkaBrokerAttackState{
				ExecutionID: uuid.New(),
				SuccessRate: 100,
			},
			executionRunData: getExecutionRunData(4, 11),
			wantedError:      extension_kit.ToError("Success Rate (36.36%) was below 100%", nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			saveExecutionRunData(tt.state.ExecutionID, tt.executionRunData)
			//When
			result, err := stop(tt.state)

			//Then
			if tt.wantedError != nil && result.Error == nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			} else if tt.wantedError != nil && result.Error != nil {
				assert.Equal(t, tt.wantedError.Error(), result.Error.Title)
			} else if tt.wantedError == nil && result.Error != nil {
				assert.Fail(t, "Should not have error", result.Error.Title)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func getExecutionRunData(successCounter uint64, counter uint64) *ExecutionRunData {
	data := &ExecutionRunData{
		requestSuccessCounter: atomic.Uint64{},
		requestCounter:        atomic.Uint64{},
	}
	data.requestCounter.Store(counter)
	data.requestSuccessCounter.Store(successCounter)
	return data

}

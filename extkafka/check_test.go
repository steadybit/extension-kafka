/*
* Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

//import (
//	"github.com/google/uuid"
//	"github.com/steadybit/action-kit/go/action_kit_api/v2"
//	extension_kit "github.com/steadybit/extension-kit"
//	"github.com/steadybit/extension-kit/extutil"
//	"github.com/stretchr/testify/assert"
//	"net/url"
//	"sync/atomic"
//	"testing"
//	"time"
//)
//
//func TestAction_Prepare(t *testing.T) {
//
//	url, _ := url.Parse("https://steadybit.com")
//
//	tests := []struct {
//		name        string
//		requestBody action_kit_api.PrepareActionRequestBody
//		wantedError error
//		wantedState *HTTPCheckState
//	}{
//		{
//			name: "Should return config",
//			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
//				Config: map[string]interface{}{
//					"action":            "prepare",
//					"duration":          5000,
//					"statusCode":        "200-209",
//					"responsesContains": "test",
//					"successRate":       100,
//					"maxConcurrent":     10,
//					"numberOfRequests":  5,
//					"readTimeout":       5000,
//					"body":              "test",
//					"url":               "https://steadybit.com",
//					"method":            "GET",
//					"connectTimeout":    5000,
//					"followRedirects":   true,
//					"headers": []any{
//						map[string]any{"key": "test", "value": "test"},
//					},
//				},
//				ExecutionId: uuid.New(),
//			}),
//
//			wantedState: &HTTPCheckState{
//				ExpectedStatusCodes:      []int{200, 201, 202, 203, 204, 205, 206, 207, 208, 209},
//				DelayBetweenRequestsInMS: 1000,
//				Timeout:                  time.Now(),
//				ResponsesContains:        "test",
//				SuccessRate:              100,
//				MaxConcurrent:            10,
//				NumberOfRecords:         5,
//				ReadTimeout:              time.Second * 5,
//				ExecutionID:              uuid.New(),
//				Body:                     "test",
//				URL:                      *url,
//				Method:                   "GET",
//				RecordHeaders:                  map[string]string{"test": "test"},
//				ConnectionTimeout:        time.Second * 5,
//				FollowRedirects:          true,
//			},
//		}, {
//			name: "Should return error for headers",
//			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
//				Config: map[string]interface{}{
//					"action":     "prepare",
//					"headers":    "test:test",
//					"statusCode": "200",
//				},
//				ExecutionId: uuid.New(),
//			}),
//
//			wantedError: extension_kit.ToError("failed to interpret config value for headers as a key/value array", nil),
//		}, {
//			name: "Should return error for missing url",
//			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
//				Config: map[string]interface{}{
//					"action":            "prepare",
//					"statusCode":        "200-209",
//					"responsesContains": "test",
//					"successRate":       100,
//					"maxConcurrent":     10,
//					"numberOfRequests":  5,
//					"readTimeout":       5000,
//					"body":              "test",
//					"method":            "GET",
//					"connectTimeout":    5000,
//					"followRedirects":   true,
//					"headers": []any{
//						map[string]any{"key": "test", "value": "test"},
//					},
//				},
//				ExecutionId: uuid.New(),
//			}),
//
//			wantedError: extension_kit.ToError("URL is missing", nil),
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			//Given
//			state := HTTPCheckState{}
//			request := tt.requestBody
//			//When
//			_, err := prepare(request, &state, func(executionRunData *ExecutionRunData, state *HTTPCheckState) bool { return false })
//
//			//Then
//			if tt.wantedError != nil {
//				assert.EqualError(t, err, tt.wantedError.Error())
//			}
//			if tt.wantedState != nil {
//				assert.NoError(t, err)
//				assert.Equal(t, tt.wantedState.FollowRedirects, state.FollowRedirects)
//				assert.Equal(t, tt.wantedState.ReadTimeout, state.ReadTimeout)
//				assert.Equal(t, tt.wantedState.FollowRedirects, state.FollowRedirects)
//				assert.Equal(t, tt.wantedState.ConnectionTimeout, state.ConnectionTimeout)
//				assert.Equal(t, tt.wantedState.ExpectedStatusCodes, state.ExpectedStatusCodes)
//				assert.Equal(t, tt.wantedState.RecordHeaders, state.RecordHeaders)
//				assert.Equal(t, tt.wantedState.MaxConcurrent, state.MaxConcurrent)
//				assert.Equal(t, tt.wantedState.Method, state.Method)
//				assert.Equal(t, tt.wantedState.NumberOfRecords, state.NumberOfRecords)
//				assert.Equal(t, tt.wantedState.ReadTimeout, state.ReadTimeout)
//				assert.Equal(t, tt.wantedState.ResponsesContains, state.ResponsesContains)
//				assert.Equal(t, tt.wantedState.SuccessRate, state.SuccessRate)
//				assert.Equal(t, tt.wantedState.URL, state.URL)
//				assert.NotNil(t, state.ExecutionID)
//				assert.NotNil(t, state.Timeout)
//				assert.EqualValues(t, tt.wantedState.Body, state.Body)
//			}
//		})
//	}
//}
//func TestAction_Stop(t *testing.T) {
//
//	tests := []struct {
//		name             string
//		requestBody      action_kit_api.StopActionRequestBody
//		state            *HTTPCheckState
//		executionRunData *ExecutionRunData
//		wantedError      error
//	}{
//		{
//			name:        "Should successfully stop the action",
//			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
//			state: &HTTPCheckState{
//				ExecutionID: uuid.New(),
//				SuccessRate: 40,
//			},
//			executionRunData: getExecutionRunData(5, 10),
//			wantedError:      nil,
//		}, {
//			name:        "Should fail because of low success rate",
//			requestBody: extutil.JsonMangle(action_kit_api.StopActionRequestBody{}),
//			state: &HTTPCheckState{
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

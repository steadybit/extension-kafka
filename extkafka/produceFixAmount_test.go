/*
* Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

//import (
//	"context"
//	"github.com/google/uuid"
//	"github.com/steadybit/action-kit/go/action_kit_api/v2"
//	extension_kit "github.com/steadybit/extension-kit"
//	"github.com/steadybit/extension-kit/extutil"
//	"github.com/stretchr/testify/assert"
//	"net/http"
//	"net/http/httptest"
//	"net/url"
//	"testing"
//	"time"
//)
//
//func TestNewHTTPCheckActionFixedAmount_Prepare(t *testing.T) {
//	action := httpCheckActionFixedAmount{}
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
//					"headers":           []interface{}{map[string]interface{}{"key": "test", "value": "test"}},
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
//					"duration":   "5000",
//					"headers":    "test:test",
//					"statusCode": "200",
//				},
//				ExecutionId: uuid.New(),
//			}),
//
//			wantedError: extension_kit.ToError("failed to interpret config value for headers as a key/value array", nil),
//		}, {
//			name: "Should return error missing duration",
//			requestBody: action_kit_api.PrepareActionRequestBody{
//				Config: map[string]interface{}{
//					"action":            "prepare",
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
//					"headers":           []interface{}{map[string]interface{}{"key": "test", "value": "test"}},
//				},
//				ExecutionId: uuid.New(),
//			},
//
//			wantedError: extension_kit.ToError("duration must be greater than 0", nil),
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			//Given
//			state := action.NewEmptyState()
//			request := tt.requestBody
//			//When
//			_, err := action.Prepare(context.Background(), &state, request)
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
//				assert.Equal(t, tt.wantedState.DelayBetweenRequestsInMS, state.DelayBetweenRequestsInMS)
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
//
//func TestNewHTTPCheckActionFixedAmount_All_Success(t *testing.T) {
//	// generate a test server so we can capture and inspect the request
//	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
//		res.Write([]byte("this is a test response"))
//		res.WriteHeader(200)
//	}))
//	defer func() { testServer.Close() }()
//
//	//prepare the action
//	action := httpCheckActionFixedAmount{}
//	state := action.NewEmptyState()
//	prepareActionRequestBody := extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
//		Config: map[string]interface{}{
//			"action":            "prepare",
//			"duration":          1000,
//			"statusCode":        "200-209",
//			"responsesContains": "test",
//			"successRate":       100,
//			"maxConcurrent":     10,
//			"numberOfRequests":  2,
//			"readTimeout":       5000,
//			"body":              "test",
//			"url":               testServer.URL,
//			"method":            "GET",
//			"connectTimeout":    5000,
//			"followRedirects":   true,
//			"headers":           []interface{}{map[string]interface{}{"key": "test", "value": "test"}},
//		},
//		ExecutionId: uuid.New(),
//	})
//
//	// Prepare
//	prepareResult, err := action.Prepare(context.Background(), &state, prepareActionRequestBody)
//	assert.NoError(t, err)
//	assert.Nil(t, prepareResult)
//	assert.Greater(t, state.DelayBetweenRequestsInMS, extutil.ToInt64(0))
//
//	executionRunData, err := action.getExecutionRunData(state.ExecutionID)
//	assert.NoError(t, err)
//	assert.NotNil(t, executionRunData)
//	// Start
//	startResult, err := action.Start(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.Nil(t, startResult)
//
//	// Status
//	statusResult, err := action.Status(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.NotNil(t, statusResult.Metrics)
//	time.Sleep(1 * time.Second)
//	// Status completed
//	statusResult, err = action.Status(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.Equal(t, statusResult.Completed, true)
//	assert.Greater(t, len(*statusResult.Metrics), 0)
//
//	assert.Equal(t, executionRunData.requestCounter.Load(), uint64(2))
//	// Stop
//	stopResult, err := action.Stop(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.NotNil(t, stopResult.Metrics)
//	assert.Nil(t, stopResult.Error)
//	assert.Equal(t, executionRunData.requestSuccessCounter.Load(), uint64(2))
//}
//
//func TestNewHTTPCheckActionFixedAmount_All_Failure(t *testing.T) {
//	// generate a test server so we can capture and inspect the request
//	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
//		res.WriteHeader(404)
//	}))
//	defer func() { testServer.Close() }()
//
//	//prepare the action
//	action := httpCheckActionFixedAmount{}
//	state := action.NewEmptyState()
//	prepareActionRequestBody := extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
//		Config: map[string]interface{}{
//			"action":            "prepare",
//			"duration":          1000,
//			"statusCode":        "200-209",
//			"responsesContains": "test",
//			"successRate":       100,
//			"maxConcurrent":     10,
//			"numberOfRequests":  2,
//			"readTimeout":       5000,
//			"body":              "test",
//			"url":               testServer.URL,
//			"method":            "GET",
//			"connectTimeout":    5000,
//			"followRedirects":   true,
//			"headers":           []interface{}{map[string]interface{}{"key": "test", "value": "test"}},
//		},
//		ExecutionId: uuid.New(),
//	})
//
//	// Prepare
//	prepareResult, err := action.Prepare(context.Background(), &state, prepareActionRequestBody)
//	assert.NoError(t, err)
//	assert.Nil(t, prepareResult)
//	assert.Greater(t, state.DelayBetweenRequestsInMS, extutil.ToInt64(0))
//
//	// Start
//	startResult, err := action.Start(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.Nil(t, startResult)
//
//	// Status
//	statusResult, err := action.Status(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.NotNil(t, statusResult.Metrics)
//	time.Sleep(1 * time.Second)
//	// Status completed
//	statusResult, err = action.Status(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.Equal(t, statusResult.Completed, true)
//	assert.Greater(t, len(*statusResult.Metrics), 0)
//
//	executionRunData, err := action.getExecutionRunData(state.ExecutionID)
//	assert.NoError(t, err)
//	assert.Greater(t, executionRunData.requestCounter.Load(), uint64(0))
//	// Stop
//	stopResult, err := action.Stop(context.Background(), &state)
//	assert.NoError(t, err)
//	assert.NotNil(t, stopResult.Metrics)
//	assert.NotNil(t, stopResult.Error)
//	assert.Equal(t, stopResult.Error.Title, "Success Rate (0.00%) was below 100%")
//	assert.Equal(t, executionRunData.requestSuccessCounter.Load(), uint64(0))
//}

/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"strings"
	"testing"
	"time"
)

func TestCheckBrokers_Describe(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name: "Should return description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := CheckBrokersAction{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "Check activity of brokers.", response.Description)
			assert.Equal(t, "Check Brokers", response.Label)
			assert.Equal(t, fmt.Sprintf("%s.check", kafkaBrokerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
	}
}

func TestCheckBrokers_Prepare(t *testing.T) {
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

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Config: map[string]interface{}{
					"expectedChanges": []string{"test"},
					"stateCheckMode":  "test",
					"duration":        10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &CheckBrokersState{
				ExpectedChanges:   []string{"test"},
				StateCheckMode:    "test",
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
				assert.Equal(t, "test", tt.wantedState.StateCheckMode)
				assert.Equal(t, []string{"test"}, state.ExpectedChanges)
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
		wantedState *CheckBrokersState
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
			name: "Should return status ok with all the time check mode",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.consumer-group.name": {"steadybit"},
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
			_, errPrepare := action.Prepare(context.TODO(), &state, request)
			statusResult, errStatus := action.Status(context.TODO(), &state)
			time.Sleep(6 * time.Second)
			err := c.RemoveNode(1)
			if err != nil {
				return
			}

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
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
				assert.Equal(t, true, statusResult.Completed)
				assert.NotNil(t, state.End)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAlterLimit_Describe(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *AlterState
	}{
		{
			name: "Should return description",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterLimitConnectionCreateRateAttack{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "Limit the Connection Creation Rate", response.Description)
			assert.Equal(t, "Limit Connection Creation Rate", response.Label)
			assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
			assert.Equal(t, fmt.Sprintf("%s.limit-connection-creation", kafkaBrokerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterMessageMaxBytesAttack{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "Reduce the max bytes allowed per message", response.Description)
			assert.Equal(t, "Reduce Message Batch Size", response.Label)
			assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
			assert.Equal(t, fmt.Sprintf("%s.reduce-message-max-bytes", kafkaBrokerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterNumberIOThreadsAttack{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "Limit the number of IO threads", response.Description)
			assert.Equal(t, "Limit IO Threads", response.Label)
			assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
			assert.Equal(t, fmt.Sprintf("%s.limit-io-threads", kafkaBrokerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterNumberNetworkThreadsAttack{}
			//When
			response := action.Describe()

			//Then
			assert.Equal(t, "Limit the number of network threads", response.Description)
			assert.Equal(t, "Limit Network Threads", response.Label)
			assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
			assert.Equal(t, fmt.Sprintf("%s.limit-network-threads", kafkaBrokerTargetId), response.Id)
			assert.Equal(t, extutil.Ptr("Kafka"), response.Technology)
		})
	}
}

func TestAlter_Prepare(t *testing.T) {
	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *AlterState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.broker.node-id": {"1"},
					},
				},
				Config: map[string]interface{}{
					"network_threads": 2,
					"io_threads":      1,
					"max_bytes":       256,
					"connection_rate": 2,
					"duration":        10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &AlterState{
				BrokerConfigValue: "network_threads",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterNumberNetworkThreadsAttack{}
			state := AlterState{}
			request := tt.requestBody
			//When
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, "network_threads", tt.wantedState.BrokerConfigValue)
			}
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterNumberIOThreadsAttack{}
			state := AlterState{}
			request := tt.requestBody
			tt.wantedState.BrokerConfigValue = "io_threads"
			//When
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, "io_threads", tt.wantedState.BrokerConfigValue)
			}
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterMessageMaxBytesAttack{}
			state := AlterState{}
			request := tt.requestBody
			tt.wantedState.BrokerConfigValue = "max_bytes"
			//When
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, "max_bytes", tt.wantedState.BrokerConfigValue)
			}
		})
		t.Run(tt.name, func(t *testing.T) {
			//Given
			action := AlterLimitConnectionCreateRateAttack{}
			state := AlterState{}
			request := tt.requestBody
			tt.wantedState.BrokerConfigValue = "connection_rate"
			//When
			_, err := action.Prepare(context.TODO(), &state, request)

			//Then
			if tt.wantedError != nil {
				assert.EqualError(t, err, tt.wantedError.Error())
			}
			if tt.wantedState != nil {
				assert.NoError(t, err)
				assert.Equal(t, "connection_rate", tt.wantedState.BrokerConfigValue)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"fmt"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAlterActions_Describe(t *testing.T) {
	t.Run("AlterLimitConnectionCreate", func(t *testing.T) {
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

	t.Run("AlterMessageMaxBytes", func(t *testing.T) {
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

	t.Run("AlterNumberIOThreads", func(t *testing.T) {
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

	t.Run("AlterNumberNetworkThreads", func(t *testing.T) {
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

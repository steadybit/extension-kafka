// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"fmt"
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
		assert.Equal(t, "Reduce the broker's max.connection.creation.rate to simulate slow acceptance of new client connections. The original value is restored when the attack ends.", response.Description)
		assert.Equal(t, "Limit Connection Creation Rate", response.Label)
		assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
		assert.Equal(t, fmt.Sprintf("%s.limit-connection-creation", kafkaBrokerTargetId), response.Id)
		assert.Equal(t, new("Kafka"), response.Technology)
	})

	t.Run("AlterMessageMaxBytes", func(t *testing.T) {
		//Given
		action := AlterMessageMaxBytesAttack{}
		//When
		response := action.Describe()

		//Then
		assert.Equal(t, "Reduce the broker's message.max.bytes to reject messages exceeding the new size limit. The original value is restored when the attack ends.", response.Description)
		assert.Equal(t, "Reduce Message Batch Size", response.Label)
		assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
		assert.Equal(t, fmt.Sprintf("%s.reduce-message-max-bytes", kafkaBrokerTargetId), response.Id)
		assert.Equal(t, new("Kafka"), response.Technology)
	})

	t.Run("AlterNumberIOThreads", func(t *testing.T) {
		//Given
		action := AlterNumberIOThreadsAttack{}
		//When
		response := action.Describe()

		//Then
		assert.Equal(t, "Reduce the broker's num.io.threads to degrade disk I/O capacity, causing increased latency or request timeouts. The original value is restored when the attack ends.", response.Description)
		assert.Equal(t, "Limit IO Threads", response.Label)
		assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
		assert.Equal(t, fmt.Sprintf("%s.limit-io-threads", kafkaBrokerTargetId), response.Id)
		assert.Equal(t, new("Kafka"), response.Technology)
	})

	t.Run("AlterNumberNetworkThreads", func(t *testing.T) {
		//Given
		action := AlterNumberNetworkThreadsAttack{}
		//When
		response := action.Describe()

		//Then
		assert.Equal(t, "Reduce the broker's num.network.threads to limit its ability to process network requests from clients. The original value is restored when the attack ends.", response.Description)
		assert.Equal(t, "Limit Network Threads", response.Label)
		assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
		assert.Equal(t, fmt.Sprintf("%s.limit-network-threads", kafkaBrokerTargetId), response.Id)
		assert.Equal(t, new("Kafka"), response.Technology)
	})
}

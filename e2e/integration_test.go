// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package e2e

import (
	"context"
	"fmt"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_test/e2e"
	actValidate "github.com/steadybit/action-kit/go/action_kit_test/validate"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_test/validate"
	"github.com/steadybit/extension-kit/extlogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os/exec"
	"testing"
	"time"
)

func TestWithMinikube(t *testing.T) {
	extlogging.InitZeroLog()

	extFactory := e2e.HelmExtensionFactory{
		Name: "extension-kafka",
		Port: 8083,
		ExtraArgs: func(m *e2e.Minikube) []string {
			return []string{
				"--set", "kafka.seedBrokers='my-kafka.default.svc.cluster.local:9092'",
				"--set", "kafka.auth.saslMechanism=PLAIN",
				"--set", "kafka.auth.saslUser=user1",
				"--set", "kafka.auth.saslPassword=steadybit",
			}
		},
	}

	e2e.WithMinikube(t, e2e.DefaultMinikubeOpts().AfterStart(helmInstallLocalStack), &extFactory, []e2e.WithMinikubeTestCase{
		{
			Name: "validate discovery",
			Test: validateDiscovery,
		},
		{
			Name: "test discovery",
			Test: testDiscovery,
		},
		{
			Name: "validate Actions",
			Test: validateActions,
		},
		{
			Name: "alter num io threads",
			Test: testAlterNumIoThreads,
		},
		{
			Name: "alter num network threads",
			Test: testAlterNumNetworkThreads,
		},
	})
}

func validateDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, validate.ValidateEndpointReferences("/", e.Client))
}

func testDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	target, err := e2e.PollForTarget(ctx, e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "1")
	})
	require.NoError(t, err)
	assert.Equal(t, target.TargetType, "com.steadybit.extension_kafka.broker")
	assert.Equal(t, target.Attributes["kafka.broker.node-id"], []string{"1"})
	assert.Equal(t, target.Attributes["kafka.broker.port"], []string{"9092"})
	assert.Equal(t, target.Attributes["kafka.broker.host"], []string{"my-kafka-controller-1.my-kafka-controller-headless.default.svc.cluster.local"})
}

func validateActions(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, actValidate.ValidateEndpointReferences("/", e.Client))
}

func testAlterNumIoThreads(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	target := &action_kit_api.Target{
		Name: "test_broker",
		Attributes: map[string][]string{
			"kafka.broker.node-id": {"0"},
		},
	}

	config := struct {
		Duration  int     `json:"duration"`
		IoThreads float32 `json:"io_threads"`
	}{
		Duration:  5000,
		IoThreads: 1.0,
	}

	// Reduce
	increaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-io-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = increaseThreadsAction.Cancel() }()

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, increaseThreadsAction.Messages())
	require.NotEmpty(t, t, increaseThreadsAction.Metrics())

	// Increase
	config.IoThreads = 100.0
	decreaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-io-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = decreaseThreadsAction.Cancel() }()

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, decreaseThreadsAction.Messages())
	require.NotEmpty(t, t, decreaseThreadsAction.Metrics())
}

func testAlterNumNetworkThreads(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	target := &action_kit_api.Target{
		Name: "test_broker",
		Attributes: map[string][]string{
			"kafka.broker.node-id": {"0"},
		},
	}

	config := struct {
		Duration       int     `json:"duration"`
		NetworkThreads float32 `json:"network_threads"`
	}{
		Duration:       5000,
		NetworkThreads: 1.0,
	}

	// Reduce
	increaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-network-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = increaseThreadsAction.Cancel() }()

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, increaseThreadsAction.Messages())
	require.NotEmpty(t, t, increaseThreadsAction.Metrics())

	// Increase
	config.NetworkThreads = 100.0
	decreaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-network-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = decreaseThreadsAction.Cancel() }()

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, decreaseThreadsAction.Messages())
	require.NotEmpty(t, t, decreaseThreadsAction.Metrics())
}

func helmInstallLocalStack(minikube *e2e.Minikube) error {
	out, err := exec.Command("helm", "repo", "add", "bitnami", "https://charts.bitnami.com/bitnami").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %s: %s", err, out)
	}
	out, err = exec.Command("helm",
		"upgrade", "--install",
		"--kube-context", minikube.Profile,
		"--set", "sasl.client.passwords=steadybit",
		"--namespace=default",
		"--timeout=15m0s",
		"my-kafka", "bitnami/kafka ", "--wait").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %s: %s", err, out)
	}
	return nil
}

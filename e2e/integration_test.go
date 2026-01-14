// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package e2e

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_test/client"
	"github.com/steadybit/action-kit/go/action_kit_test/e2e"
	actValidate "github.com/steadybit/action-kit/go/action_kit_test/validate"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_test/validate"
	"github.com/steadybit/extension-kit/extlogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var kafkactl func(ctx context.Context, commands ...string) (string, error)
var kafkactlStop func()

func TestWithMinikube(t *testing.T) {
	extlogging.InitZeroLog()

	extFactory := e2e.HelmExtensionFactory{
		Name: "extension-kafka",
		Port: 8083,

		ExtraArgs: func(m *e2e.Minikube) []string {
			return []string{
				"--set", "logging.level=debug",
				// Multi-cluster configuration
				"--set", "kafka.clusters[0].name=cluster-1",
				"--set", "kafka.clusters[0].seedBrokers=my-kafka.default.svc.cluster.local:9092",
				"--set", "kafka.clusters[0].auth.saslMechanism=PLAIN",
				"--set", "kafka.clusters[0].auth.saslUser=user1",
				"--set", "kafka.clusters[0].auth.saslPassword=steadybit",
				"--set", "kafka.clusters[1].name=cluster-2",
				"--set", "kafka.clusters[1].seedBrokers=my-kafka-2.default.svc.cluster.local:9092",
				"--set", "kafka.clusters[1].auth.saslMechanism=PLAIN",
				"--set", "kafka.clusters[1].auth.saslUser=user1",
				"--set", "kafka.clusters[1].auth.saslPassword=steadybit",
			}
		},
	}

	defer func() {
		if kafkactlStop != nil {
			kafkactlStop()
		}
	}()

	e2e.WithMinikube(t, e2e.DefaultMinikubeOpts().AfterStart(helmInstallLocalStack).AfterStart(setupKafkactl), &extFactory, []e2e.WithMinikubeTestCase{
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
		{
			Name: "alter limit connection creation rate",
			Test: testAlterLimitConnectionCreationRate,
		},
		{
			Name: "alter max message bytes",
			Test: testAlterMaxMessageBytes,
		},
	})
}

func validateDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, validate.ValidateEndpointReferences("/", e.Client))
}

func testDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Verify broker from first cluster is discovered
	target1, err := e2e.PollForTarget(ctx, e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "1") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-controller")
	})
	require.NoError(t, err)
	assert.Equal(t, target1.TargetType, "com.steadybit.extension_kafka.broker")
	assert.Equal(t, target1.Attributes["kafka.broker.node-id"], []string{"1"})
	assert.Equal(t, target1.Attributes["kafka.broker.port"], []string{"9092"})
	assert.Equal(t, target1.Attributes["kafka.broker.host"], []string{"my-kafka-controller-1.my-kafka-controller-headless.default.svc.cluster.local"})
	cluster1Name := target1.Attributes["kafka.cluster.name"][0]
	require.NotEmpty(t, cluster1Name, "cluster name should be set for first cluster")

	// Verify broker from second cluster is discovered (it has only 1 replica, so node-id is 0)
	target2, err := e2e.PollForTarget(ctx, e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "0") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-2-controller")
	})
	require.NoError(t, err)
	assert.Equal(t, target2.TargetType, "com.steadybit.extension_kafka.broker")
	assert.Equal(t, target2.Attributes["kafka.broker.node-id"], []string{"0"})
	assert.Equal(t, target2.Attributes["kafka.broker.port"], []string{"9092"})
	assert.Equal(t, target2.Attributes["kafka.broker.host"], []string{"my-kafka-2-controller-0.my-kafka-2-controller-headless.default.svc.cluster.local"})
	cluster2Name := target2.Attributes["kafka.cluster.name"][0]
	require.NotEmpty(t, cluster2Name, "cluster name should be set for second cluster")

	// Verify the two clusters have different names
	assert.NotEqual(t, cluster1Name, cluster2Name, "cluster names should be different for different Kafka clusters")
	t.Logf("Discovered brokers from two different clusters: %s and %s", cluster1Name, cluster2Name)
}

func validateActions(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, actValidate.ValidateEndpointReferences("/", e.Client))
}

func testAlterNumIoThreads(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	// Discover a target to get the cluster name - must be from first cluster (my-kafka) since kafkactl is configured for it
	discoveredTarget, err := e2e.PollForTarget(t.Context(), e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "0") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-controller-")
	})
	require.NoError(t, err)

	target := &action_kit_api.Target{
		Name: "test_broker",
		Attributes: map[string][]string{
			"kafka.broker.node-id": {"0"},
			"kafka.cluster.name":   discoveredTarget.Attributes["kafka.cluster.name"],
		},
	}

	config := struct {
		Duration  int     `json:"duration"`
		IoThreads float32 `json:"io_threads"`
	}{
		Duration:  20000,
		IoThreads: 1.0,
	}

	// Reduce
	increaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-io-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = increaseThreadsAction.Cancel() }()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		brokerConfig, err := kafkactl(t.Context(), "describe", "broker", "0")
		assert.NoError(c, err, "Failed to describe broker config")
		assert.Regexp(c, `num\.io\.threads\s+1`, brokerConfig, "property not found")
	}, 20*time.Second, 1*time.Second, "num.io.threads should be set to 1")

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, increaseThreadsAction.Messages())
	require.NotEmpty(t, t, increaseThreadsAction.Metrics())

	// Increase
	config.IoThreads = 100.0
	decreaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-io-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = decreaseThreadsAction.Cancel() }()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		brokerConfig, err := kafkactl(t.Context(), "describe", "broker", "0")
		assert.NoError(c, err, "Failed to describe broker config")
		assert.Regexp(c, `num\.io\.threads\s+1`, brokerConfig, "property not found")
	}, 20*time.Second, 1*time.Second, "num.io.threads should be set to 100")

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, decreaseThreadsAction.Messages())
	require.NotEmpty(t, t, decreaseThreadsAction.Metrics())
}

func testAlterNumNetworkThreads(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	// Discover a target to get the cluster name - must be from first cluster (my-kafka) since kafkactl is configured for it
	discoveredTarget, err := e2e.PollForTarget(t.Context(), e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "0") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-controller-")
	})
	require.NoError(t, err)

	target := &action_kit_api.Target{
		Name: "test_broker",
		Attributes: map[string][]string{
			"kafka.broker.node-id": {"0"},
			"kafka.cluster.name":   discoveredTarget.Attributes["kafka.cluster.name"],
		},
	}

	config := struct {
		Duration       int `json:"duration"`
		NetworkThreads int `json:"network_threads"`
	}{
		Duration:       20000,
		NetworkThreads: 1,
	}

	// Reduce
	increaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-network-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = increaseThreadsAction.Cancel() }()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		brokerConfig, err := kafkactl(t.Context(), "describe", "broker", "0")
		assert.NoError(c, err, "Failed to describe broker config")
		assert.Regexp(c, `num\.network\.threads\s+1`, brokerConfig, "property not found")
	}, 20*time.Second, 1*time.Second, "num.network.threads should be set to 1")

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, increaseThreadsAction.Messages())
	require.NotEmpty(t, t, increaseThreadsAction.Metrics())

	// Increase
	config.NetworkThreads = 100.0
	decreaseThreadsAction, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-network-threads", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = decreaseThreadsAction.Cancel() }()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		brokerConfig, err := kafkactl(t.Context(), "describe", "broker", "0")
		assert.NoError(c, err, "Failed to describe broker config")
		assert.Regexp(c, `num\.network\.threads\s+1`, brokerConfig, "property not found")
	}, 20*time.Second, 1*time.Second, "num.network.threads should be set to 1")

	require.NoError(t, increaseThreadsAction.Wait())
	require.NotEmpty(t, t, decreaseThreadsAction.Messages())
	require.NotEmpty(t, t, decreaseThreadsAction.Metrics())
}

func testAlterLimitConnectionCreationRate(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	// Discover a target to get the cluster name - must be from first cluster (my-kafka) since kafkactl is configured for it
	discoveredTarget, err := e2e.PollForTarget(t.Context(), e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "0") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-controller-")
	})
	require.NoError(t, err)

	target := &action_kit_api.Target{
		Name: "test_broker",
		Attributes: map[string][]string{
			"kafka.broker.node-id": {"0"},
			"kafka.cluster.name":   discoveredTarget.Attributes["kafka.cluster.name"],
		},
	}

	config := struct {
		Duration       int `json:"duration"`
		ConnectionRate int `json:"connection_rate"`
	}{
		Duration:       20000,
		ConnectionRate: 1,
	}

	action, err := e.RunAction("com.steadybit.extension_kafka.broker.limit-connection-creation", target, config, &action_kit_api.ExecutionContext{})
	require.NoError(t, err)
	defer func() { _ = action.Cancel() }()

	// Testing this setting by opening up too many connections does not work with the current setup,
	// as executing "kubectl" commands is too slow, and it does not support parallel invocations.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		brokerConfig, err := kafkactl(t.Context(), "describe", "broker", "0")
		assert.NoError(c, err, "Failed to describe broker config")
		assert.Regexp(c, `max\.connection\.creation\.rate\s+1`, brokerConfig, "property not found")
	}, 20*time.Second, 1*time.Second, "max.connection.creation.rate should be set to 1")

	require.NoError(t, action.Wait())
	require.NotEmpty(t, t, action.Messages())
	require.NotEmpty(t, t, action.Metrics())
}

func testAlterMaxMessageBytes(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	message := fmt.Sprintf("{\"a\": \"%s\"}", strings.Repeat("x", 1000))
	out, err := kafkactl(t.Context(), "produce", "foo", "-v", message)
	require.NoError(t, err, out)

	config := struct {
		Duration int `json:"duration"`
		MaxBytes int `json:"max_bytes"`
	}{
		Duration: 20000,
		MaxBytes: 100,
	}

	// Discover a target to get the cluster name - must be from first cluster (my-kafka) since kafkactl is configured for it
	discoveredTarget, err := e2e.PollForTarget(t.Context(), e, "com.steadybit.extension_kafka.broker", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "kafka.broker.node-id", "0") &&
			strings.Contains(target.Attributes["kafka.broker.host"][0], "my-kafka-controller-")
	})
	require.NoError(t, err)

	// Change message size setting on all nodes (first cluster has 3 brokers)
	var action client.ActionExecution
	for i := 0; i < 3; i++ {
		target := &action_kit_api.Target{
			Name: "test_broker",
			Attributes: map[string][]string{
				"kafka.broker.node-id": {strconv.Itoa(i)},
				"kafka.cluster.name":   discoveredTarget.Attributes["kafka.cluster.name"],
			},
		}
		action, err = e.RunAction("com.steadybit.extension_kafka.broker.reduce-message-max-bytes", target, config, &action_kit_api.ExecutionContext{})
		require.NoError(t, err)
		//goland:noinspection ALL
		defer func(a client.ActionExecution) { _ = a.Cancel() }(action)
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err = kafkactl(t.Context(), "produce", "foo", "-v", message)
		require.Error(t, err, out)
	}, 20*time.Second, 1*time.Second, "long messages should be rejected")

	//goland:noinspection GoDfaNilDereference
	require.NoError(t, action.Wait())
	require.NotEmpty(t, t, action.Messages())
	require.NotEmpty(t, t, action.Metrics())
}

func helmInstallLocalStack(minikube *e2e.Minikube) error {
	out, err := exec.Command("helm", "repo", "add", "bitnami", "https://charts.bitnami.com/bitnami").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %s: %s", err, out)
	}

	// Install first Kafka cluster
	out, err = exec.Command("helm",
		"upgrade", "--install",
		"--kube-context", minikube.Profile,
		"--set", "sasl.client.passwords=steadybit",
		"--set", "provisioning.enabled=true",
		"--set", "provisioning.topics[0].name=foo",
		"--set", "image.repository=bitnamilegacy/kafka",
		"--set", "image.tag=4.0.0-debian-12-r10",
		"--set", "global.security.allowInsecureImages=true",
		"--namespace=default",
		"--timeout=15m0s",
		"my-kafka", "bitnami/kafka", "--wait").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install first kafka helm chart: %s: %s", err, out)
	}

	// Install second Kafka cluster
	out, err = exec.Command("helm",
		"upgrade", "--install",
		"--kube-context", minikube.Profile,
		"--set", "sasl.client.passwords=steadybit",
		"--set", "provisioning.enabled=true",
		"--set", "provisioning.topics[0].name=bar",
		"--set", "image.repository=bitnamilegacy/kafka",
		"--set", "image.tag=4.0.0-debian-12-r10",
		"--set", "global.security.allowInsecureImages=true",
		"--set", "controller.replicaCount=1",
		"--namespace=default",
		"--timeout=15m0s",
		"my-kafka-2", "bitnami/kafka", "--wait").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install second kafka helm chart: %s: %s", err, out)
	}

	return nil
}

func setupKafkactl(m *e2e.Minikube) error {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "kafkactl", "config.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	backupPath := configPath + ".backup"
	if _, err := os.Stat(configPath); err == nil {
		if err := copyFile(configPath, backupPath); err != nil {
			return fmt.Errorf("failed to backup config: %w", err)
		}
	}

	stop := func() {
		if err := os.Rename(backupPath, configPath); err != nil {
			log.Error().Err(err).Msg("Failed to restore original config")
		}
	}

	configContent := fmt.Sprintf(`contexts:
  e2e:
    brokers:
      - my-kafka.default.svc.cluster.local:9092
    tls:
      enabled: false
    sasl:
      enabled: true
      username: user1
      password: steadybit
    kubernetes:
      enabled: true
      kubecontext: %s
      namespace: default
`, m.Profile)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		stop()
		return fmt.Errorf("failed to write temporary config: %w", err)
	}

	kafkactl = func(ctx context.Context, commands ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "kafkactl", append(commands, "--context", "e2e")...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("kafkactl command failed: %w, output: %s", err, string(output))
		}
		return string(output), nil
	}
	kafkactlStop = stop
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(sourceFile *os.File) {
		_ = sourceFile.Close()
	}(sourceFile)

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func(destFile *os.File) {
		_ = destFile.Close()
	}(destFile)

	_, err = io.Copy(destFile, sourceFile)
	return err
}

/*
 * Copyright 2024 steadybit GmbH. All rights reserved.
 */

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024 Steadybit GmbH

package e2e

import (
	"context"
	"fmt"
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
	})
}

func validateDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, validate.ValidateEndpointReferences("/", e.Client))
}

func testDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

func helmInstallLocalStack(minikube *e2e.Minikube) error {
	out, err := exec.Command("helm", "repo", "add", "bitnami", "https://charts.bitnami.com/bitnami").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %s: %s", err, out)
	}
	out, err = exec.Command("helm",
		"upgrade", "--install",
		"--kube-context", minikube.Profile,
		"--set", "sasl.client.passwords=steadybit",
		"--set", "controller.replicaCount=1",
		"--namespace=default",
		"--timeout=15m0s",
		"my-kafka", "bitnami/kafka ", "--wait").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install helm chart: %s: %s", err, out)
	}
	return nil
}

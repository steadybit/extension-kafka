// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2023 Steadybit GmbH

package e2e

import (
	"context"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_test/e2e"
	"github.com/steadybit/discovery-kit/go/discovery_kit_api"
	"github.com/steadybit/discovery-kit/go/discovery_kit_test/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestWithMinikube(t *testing.T) {
	extFactory := e2e.HelmExtensionFactory{
		Name: "extension-kafka",
		Port: 8080,
		ExtraArgs: func(m *e2e.Minikube) []string {
			return []string{
				"--set", "logging.level=debug",
				"--set", "discovery.attributes.excludes.robot={robot.tags.*}",
			}
		},
	}

	e2e.WithDefaultMinikube(t, &extFactory, []e2e.WithMinikubeTestCase{
		{
			Name: "validate discovery",
			Test: validateDiscovery,
		},
		{
			Name: "target discovery",
			Test: testDiscovery,
		},
		{
			Name: "run scaffold",
			Test: testRunscaffold,
		},
	})
}

func validateDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	assert.NoError(t, validate.ValidateEndpointReferences("/", e.Client))
}

func testDiscovery(t *testing.T, _ *e2e.Minikube, e *e2e.Extension) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	target, err := e2e.PollForTarget(ctx, e, "com.steadybit.extension_scaffold.robot", func(target discovery_kit_api.Target) bool {
		return e2e.HasAttribute(target, "steadybit.label", "Bender")
	})

	require.NoError(t, err)
	assert.Equal(t, target.TargetType, "com.steadybit.extension_scaffold.robot")
	assert.Equal(t, target.Attributes["robot.reportedBy"], []string{"extension-kafka"})
	assert.NotContains(t, target.Attributes, "robot.tags.firstTag")
}

func testRunscaffold(t *testing.T, m *e2e.Minikube, e *e2e.Extension) {
	config := struct {
		Duration int `json:"duration"`
	}{
		Duration: 3000,
	}
	exec, err := e.RunAction("com.steadybit.extension_scaffold.robot.log", &action_kit_api.Target{
		Name: "robot",
	}, config, nil)
	require.NoError(t, err)
	e2e.AssertLogContains(t, m, e.Pod, "Logging in log action **start**")
	require.NoError(t, exec.Cancel())
}

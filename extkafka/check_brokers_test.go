// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestCheckBrokers_Describe(t *testing.T) {
	//Given
	action := CheckBrokersAction{}

	//When
	response := action.Describe()

	//Then
	assert.Equal(t, "Monitor broker-level changes such as controller elections and broker downtime during an experiment. For topic partition changes, use Check Partitions instead. For consumer group state, use Check Consumer State.", response.Description)
	assert.Equal(t, "Check Brokers", response.Label)
	assert.Equal(t, fmt.Sprintf("%s.check", kafkaBrokerTargetId), response.Id)
	assert.Equal(t, new("Kafka"), response.Technology)
	require.NotNil(t, response.TargetSelection)
	assert.Equal(t, kafkaBrokerTargetId, response.TargetSelection.TargetType)
}

func TestCheckBrokers_Prepare(t *testing.T) {
	c, err := kfake.NewCluster(
		kfake.SeedTopics(-1, "steadybit"),
		kfake.NumBrokers(3),
	)
	require.NoError(t, err)
	defer c.Close()

	seeds := c.ListenAddrs()
	seedBrokers := strings.Join(seeds, ",")

	// Initialize cluster configuration for test
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {
			SeedBrokers: seedBrokers,
		},
	})

	tests := []struct {
		name        string
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name: "Should return config",
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]any{
					"expectedChanges": []string{"test"},
					"changeCheckMode": "allTheTime",
					"duration":        10000,
				},
				ExecutionId: uuid.New(),
			}),

			wantedState: &CheckBrokersState{
				ExpectedChanges:   []string{"test"},
				StateCheckMode:    "allTheTime",
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
				assert.Equal(t, tt.wantedState.ExpectedChanges, state.ExpectedChanges)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.Equal(t, tt.wantedState.StateCheckSuccess, state.StateCheckSuccess)
				assert.True(t, state.FailEarly) // defaults to true when not provided (non-breaking)
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
	require.NoError(t, err)
	defer c.Close()

	seeds := c.ListenAddrs()
	seedBrokers := strings.Join(seeds, ",")

	// Initialize cluster configuration for test
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		"test-cluster": {
			SeedBrokers: seedBrokers,
		},
	})
	// One client can both produce and consume!
	// Consuming can either be direct (no consumer group), or through a group. Below, we use a group.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumerGroup("steadybit"),
		kgo.ConsumeTopics("steadybit"),
	)
	require.NoError(t, err)
	defer cl.Close()

	tests := []struct {
		name        string
		killNode    *int
		requestBody action_kit_api.PrepareActionRequestBody
		wantedError error
		wantedState *CheckBrokersState
	}{
		{
			name:     "Should return status ok",
			killNode: new(1),
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]any{
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
			name:     "Should return status ok with all the time check mode",
			killNode: new(2),
			requestBody: extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
				Target: &action_kit_api.Target{
					Attributes: map[string][]string{
						"kafka.cluster.name": {"test-cluster"},
					},
				},
				Config: map[string]any{
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
			_, errPrepare := action.Prepare(t.Context(), &state, request)
			statusResult, errStatus := action.Status(t.Context(), &state)

			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.False(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}

			if tt.wantedError != nil {
				err := c.RemoveNode(int32(*tt.killNode))
				require.NoError(t, err)
			}
			time.Sleep(6 * time.Second)

			// Completed
			statusResult, errStatus = action.Status(t.Context(), &state)
			//Then
			if tt.wantedState != nil {
				assert.NoError(t, errPrepare)
				assert.NoError(t, errStatus)
				assert.Equal(t, tt.wantedState.StateCheckMode, state.StateCheckMode)
				assert.True(t, statusResult.Completed)
				assert.NotNil(t, state.End)
			}
		})
	}
}

const cb_cluster = "cb-cluster"

// cb_prepared starts an in-memory Kafka with three brokers, registers it as cb_cluster and returns a
// state prepared with cfg.
func cb_prepared(t *testing.T, cfg map[string]any) (*kfake.Cluster, *CheckBrokersState) {
	t.Helper()
	c, err := kfake.NewCluster(kfake.NumBrokers(3))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	config.SetClustersForTest(map[string]*config.ClusterConfig{
		cb_cluster: {SeedBrokers: strings.Join(c.ListenAddrs(), ",")},
	})
	action := NewBrokersCheckAction()
	state := action.NewEmptyState()
	_, err = action.Prepare(t.Context(), &state, extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target:      &action_kit_api.Target{Attributes: map[string][]string{"kafka.cluster.name": {cb_cluster}}},
		Config:      cfg,
		ExecutionId: uuid.New(),
	}))
	require.NoError(t, err)
	return c, &state
}

func cb_metric(t *testing.T, result *action_kit_api.StatusResult) map[string]string {
	t.Helper()
	require.NotNil(t, result.Metrics)
	require.Len(t, *result.Metrics, 1)
	return (*result.Metrics)[0].Metric
}

func TestCheckBrokers_Prepare_records_the_cluster(t *testing.T) {
	c, state := cb_prepared(t, map[string]any{"duration": 60000, "failEarly": false})

	assert.Equal(t, cb_cluster, state.ClusterName)
	assert.Equal(t, c.ListenAddrs(), state.BrokerHosts)
	assert.Len(t, state.BrokerNodes, 3)
	assert.Contains(t, state.BrokerNodes, state.PreviousController, "the controller is one of the brokers")
	assert.False(t, state.FailEarly)
	assert.Empty(t, state.ExpectedChanges)
	assert.Equal(t, CheckBrokersState{}, NewBrokersCheckAction().NewEmptyState())
}

func TestCheckBrokers_Prepare_rejects_an_unknown_cluster(t *testing.T) {
	state := CheckBrokersState{}
	_, err := (&CheckBrokersAction{}).Prepare(t.Context(), &state, extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target:      &action_kit_api.Target{Attributes: map[string][]string{"kafka.cluster.name": {"unknown-cluster"}}},
		Config:      map[string]any{"duration": 1000},
		ExecutionId: uuid.New(),
	}))

	assert.ErrorContains(t, err, "failed to get cluster config")
}

func TestCheckBrokers_Start_without_changes(t *testing.T) {
	_, state := cb_prepared(t, map[string]any{"duration": 60000})

	result, err := (&CheckBrokersAction{}).Start(t.Context(), state)

	require.NoError(t, err)
	assert.Nil(t, result.Error)
	require.NotNil(t, result.Metrics)
	metric := (*result.Metrics)[0].Metric
	assert.Equal(t, "info", metric["state"])
	assert.Equal(t, "No changes", metric["tooltip"])
	assert.Equal(t, "cb-cluster - Expected: No changes", metric["metric.id"])
}

func TestCheckBrokers_detects_controller_change_and_broker_downtime(t *testing.T) {
	_, state := cb_prepared(t, map[string]any{
		"duration":        60000,
		"expectedChanges": []string{BrokerControllerChanged, BrokerDowntime},
		"changeCheckMode": stateCheckModeAllTheTime,
	})
	actualController := state.PreviousController
	// Pretend the previous poll saw another controller and a broker that is gone now.
	state.PreviousController = 999
	state.BrokerNodes = append(state.BrokerNodes, 42)

	result, err := BrokerCheckStatus(t.Context(), state)

	require.NoError(t, err)
	assert.Nil(t, result.Error, "both changes are expected")
	metric := cb_metric(t, result)
	assert.Equal(t, "success", metric["state"])
	assert.Contains(t, metric["tooltip"], fmt.Sprintf("%s: %d\n", BrokerControllerChanged, actualController))
	assert.Contains(t, metric["tooltip"], BrokerDowntime+": 42\n")
	assert.Equal(t, "cb-cluster - Expected: kafka broker with downtime,kafka controller changed", metric["metric.id"])
	assert.Equal(t, actualController, state.PreviousController)
}

func TestCheckBrokers_all_the_time_fails_early(t *testing.T) {
	t.Run("on an unexpected change", func(t *testing.T) {
		_, state := cb_prepared(t, map[string]any{
			"duration":        60000,
			"expectedChanges": []string{BrokerDowntime},
			"changeCheckMode": stateCheckModeAllTheTime,
		})
		state.PreviousController = 999

		result, err := BrokerCheckStatus(t.Context(), state)

		require.NoError(t, err)
		require.NotNil(t, result.Error)
		assert.Equal(t, "Brokers got an unexpected change 'kafka controller changed' whereas '[kafka broker with downtime]' is expected.", result.Error.Title)
		assert.Equal(t, "warn", cb_metric(t, result)["state"])
	})

	t.Run("when the expected change does not happen", func(t *testing.T) {
		_, state := cb_prepared(t, map[string]any{
			"duration":        60000,
			"expectedChanges": []string{BrokerDowntime},
			"changeCheckMode": stateCheckModeAllTheTime,
		})

		result, err := BrokerCheckStatus(t.Context(), state)

		require.NoError(t, err)
		require.NotNil(t, result.Error)
		assert.Equal(t, "Brokers got an unexpected change 'No changes' whereas '[kafka broker with downtime]' is expected.", result.Error.Title)
	})

	t.Run("on any change when no change is expected", func(t *testing.T) {
		_, state := cb_prepared(t, map[string]any{"duration": 60000})
		state.BrokerNodes = append(state.BrokerNodes, 42)

		result, err := BrokerCheckStatus(t.Context(), state)

		require.NoError(t, err)
		require.NotNil(t, result.Error)
		assert.Equal(t, "Brokers got an unexpected change '[kafka broker with downtime]' whereas 'No changes' is expected.", result.Error.Title)
		assert.Equal(t, "danger", cb_metric(t, result)["state"])
	})
}

func TestCheckBrokers_fail_at_end_reports_the_first_deviation(t *testing.T) {
	_, state := cb_prepared(t, map[string]any{"duration": 60000, "failEarly": false})

	state.BrokerNodes = append(state.BrokerNodes, 42)
	result, err := BrokerCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Nil(t, result.Error, "the step is not over yet")
	assert.True(t, state.DeviationSeen)
	firstTitle := state.DeviationTitle

	state.PreviousController = 999
	_, err = BrokerCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Equal(t, firstTitle, state.DeviationTitle, "a later deviation does not overwrite the first one")

	state.End = time.Now().Add(-time.Second)
	result, err = BrokerCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.True(t, result.Completed)
	require.NotNil(t, result.Error)
	assert.Equal(t, firstTitle, result.Error.Title)
}

func TestCheckBrokers_at_least_once_fails_at_the_end_without_the_expected_change(t *testing.T) {
	_, state := cb_prepared(t, map[string]any{
		"duration":        60000,
		"expectedChanges": []string{BrokerControllerChanged},
		"changeCheckMode": stateCheckModeAtLeastOnce,
	})
	state.BrokerNodes = append(state.BrokerNodes, 42) // a change, but not the expected one

	result, err := BrokerCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.Nil(t, result.Error)
	assert.False(t, state.StateCheckSuccess)

	state.End = time.Now().Add(-time.Second)
	result, err = BrokerCheckStatus(t.Context(), state)
	require.NoError(t, err)
	assert.True(t, result.Completed)
	require.NotNil(t, result.Error)
	assert.Equal(t, "Brokers didn't get the expected changes '[kafka controller changed]' at least once.", result.Error.Title)
}

func TestCheckBrokers_status_errors(t *testing.T) {
	t.Run("unknown cluster", func(t *testing.T) {
		result, err := BrokerCheckStatus(t.Context(), &CheckBrokersState{ClusterName: "unknown-cluster"})

		assert.Nil(t, result)
		assert.ErrorContains(t, err, "failed to get cluster config")
	})

	t.Run("unreachable cluster", func(t *testing.T) {
		c, state := cb_prepared(t, map[string]any{"duration": 60000})
		c.Close()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()

		result, err := (&CheckBrokersAction{}).Start(ctx, state)

		assert.Nil(t, result)
		assert.ErrorContains(t, err, "Failed to retrieve brokers from Kafka")
	})
}

func TestCheckBrokers_slice_helpers(t *testing.T) {
	assert.True(t, areSlicesEqualUnordered([]int32{3, 1, 2}, []int32{1, 2, 3}))
	assert.False(t, areSlicesEqualUnordered([]int32{1, 2}, []int32{1, 2, 3}))
	assert.False(t, areSlicesEqualUnordered([]int32{1, 2, 4}, []int32{1, 2, 3}))

	assert.Equal(t, []int32{4, 5}, findMissingElements([]int32{1, 4, 2, 5}, []int32{1, 2}))
	assert.Empty(t, findMissingElements([]int32{1, 2}, []int32{2, 1, 3}))

	assert.Equal(t, "1,22,-3", joinInt32s([]int32{1, 22, -3}))
	assert.Equal(t, "", joinInt32s(nil))
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// du_aclFake answers CreateACLs and DeleteACLs, keeping the created ACLs so a test can check what the
// action asked for. errCode, when set, is returned for every creation or deletion.
type du_aclFake struct {
	mu      sync.Mutex
	created []kmsg.CreateACLsRequestCreation
	deleted []kmsg.DeleteACLsRequestFilter
	errCode int16
}

func du_installACLFake(t *testing.T, c *kfake.Cluster) *du_aclFake {
	t.Helper()
	el_advertiseKeys(t, c, map[int16]int16{
		int16(kmsg.CreateACLs): 3,
		int16(kmsg.DeleteACLs): 3,
	})
	f := &du_aclFake{}
	c.ControlKey(int16(kmsg.CreateACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		r := req.(*kmsg.CreateACLsRequest)
		resp := r.ResponseKind().(*kmsg.CreateACLsResponse)
		resp.Version = r.Version
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, creation := range r.Creations {
			f.created = append(f.created, creation)
			res := kmsg.NewCreateACLsResponseResult()
			res.ErrorCode = f.errCode
			if f.errCode != 0 {
				res.ErrorMessage = kmsg.StringPtr(": denied by test")
			}
			resp.Results = append(resp.Results, res)
		}
		return resp, nil, true
	})
	c.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		c.KeepControl()
		r := req.(*kmsg.DeleteACLsRequest)
		resp := r.ResponseKind().(*kmsg.DeleteACLsResponse)
		resp.Version = r.Version
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, filter := range r.Filters {
			f.deleted = append(f.deleted, filter)
			res := kmsg.NewDeleteACLsResponseResult()
			res.ErrorCode = f.errCode
			if f.errCode != 0 {
				res.ErrorMessage = kmsg.StringPtr(": denied by test")
			}
			resp.Results = append(resp.Results, res)
		}
		return resp, nil, true
	})
	return f
}

func du_prepareRequest(attributes map[string][]string) action_kit_api.PrepareActionRequestBody {
	return extutil.JsonMangle(action_kit_api.PrepareActionRequestBody{
		Target: &action_kit_api.Target{Attributes: attributes},
		Config: map[string]any{
			"duration": 10000,
			"user":     "alice",
			"topic":    "steadybit",
		},
		ExecutionId: uuid.New(),
	})
}

func du_preparedState(t *testing.T) (*KafkaConsumerDenyAccessAttack, KafkaDenyUserState) {
	t.Helper()
	action := NewKafkaConsumerDenyAccessAttack().(*KafkaConsumerDenyAccessAttack)
	state := action.NewEmptyState()
	_, err := action.Prepare(context.Background(), &state, du_prepareRequest(map[string][]string{
		"kafka.cluster.name":        {"test-cluster"},
		"kafka.consumer-group.name": {"steadybit-group"},
	}))
	require.NoError(t, err)
	return action, state
}

func TestDenyUser_Describe(t *testing.T) {
	action := NewKafkaConsumerDenyAccessAttack()

	desc := action.Describe()

	assert.Equal(t, fmt.Sprintf("%s.deny-access", kafkaConsumerTargetId), desc.Id)
	assert.Equal(t, "Deny Access", desc.Label)
	assert.Equal(t, action_kit_api.TimeControlExternal, desc.TimeControl)
	assert.Equal(t, action_kit_api.Attack, desc.Kind)
	require.NotNil(t, desc.TargetSelection)
	assert.Equal(t, kafkaConsumerTargetId, desc.TargetSelection.TargetType)
	var names []string
	for _, p := range desc.Parameters {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"duration", "user", "topic"}, names)
	assert.Equal(t, KafkaDenyUserState{}, action.NewEmptyState())
}

func TestDenyUser_Prepare(t *testing.T) {
	c := el_newCluster(t, "steadybit", 1)

	_, state := du_preparedState(t)

	assert.Equal(t, KafkaDenyUserState{
		ConsumerGroup: "steadybit-group",
		Topic:         "steadybit",
		User:          "alice",
		BrokerHosts:   c.ListenAddrs(),
		ClusterName:   "test-cluster",
	}, state)
}

func TestDenyUser_Prepare_errors(t *testing.T) {
	config.SetClustersForTest(map[string]*config.ClusterConfig{})
	action := NewKafkaConsumerDenyAccessAttack()

	tests := []struct {
		name       string
		attributes map[string][]string
		wantErr    string
	}{
		{name: "missing consumer group", attributes: map[string][]string{"kafka.cluster.name": {"test-cluster"}}, wantErr: "missing the kafka.consumer-group.name attribute"},
		{name: "unknown cluster", attributes: map[string][]string{"kafka.cluster.name": {"missing"}, "kafka.consumer-group.name": {"g"}}, wantErr: "failed to get cluster config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := action.NewEmptyState()
			_, err := action.Prepare(context.Background(), &state, du_prepareRequest(tt.attributes))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDenyUser_Start_and_Stop_manage_deny_ACLs(t *testing.T) {
	c := el_newCluster(t, "steadybit", 1)
	fake := du_installACLFake(t, c)
	action, state := du_preparedState(t)

	result, err := action.Start(context.Background(), &state)

	require.NoError(t, err)
	require.NotNil(t, result.Messages)
	assert.Contains(t, (*result.Messages)[0].Message, "steadybit-group")

	fake.mu.Lock()
	created := fake.created
	fake.mu.Unlock()
	// read, write and describe, each on the topic and on the consumer group
	require.Len(t, created, 6)
	resources := map[string]int{}
	for _, acl := range created {
		assert.Equal(t, "User:alice", acl.Principal)
		assert.Equal(t, "*", acl.Host)
		assert.Equal(t, kmsg.ACLPermissionTypeDeny, acl.PermissionType)
		assert.Equal(t, kmsg.ACLResourcePatternTypeLiteral, acl.ResourcePatternType)
		resources[acl.ResourceType.String()+"/"+acl.ResourceName]++
	}
	assert.Equal(t, map[string]int{"TOPIC/steadybit": 3, "GROUP/steadybit-group": 3}, resources)

	stopResult, err := action.Stop(context.Background(), &state)

	require.NoError(t, err)
	assert.Nil(t, stopResult)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.deleted, 6)
	for _, filter := range fake.deleted {
		require.NotNil(t, filter.Principal)
		assert.Equal(t, "User:alice", *filter.Principal)
		assert.Equal(t, kmsg.ACLPermissionTypeDeny, filter.PermissionType)
	}
}

func TestDenyUser_Start_fails_when_ACL_creation_fails(t *testing.T) {
	c := el_newCluster(t, "steadybit", 1)
	fake := du_installACLFake(t, c)
	fake.errCode = kerr.SecurityDisabled.Code
	action, state := du_preparedState(t)

	result, err := action.Start(context.Background(), &state)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), kerr.SecurityDisabled.Message)
	assert.Contains(t, err.Error(), "denied by test")
}

func TestDenyUser_Stop_fails_when_ACL_deletion_fails(t *testing.T) {
	c := el_newCluster(t, "steadybit", 1)
	fake := du_installACLFake(t, c)
	fake.errCode = kerr.SecurityDisabled.Code
	action, state := du_preparedState(t)

	result, err := action.Stop(context.Background(), &state)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), kerr.SecurityDisabled.Message)
}

func TestDenyUser_Start_and_Stop_fail_when_ACLs_are_not_supported(t *testing.T) {
	el_newCluster(t, "steadybit", 1)
	action, state := du_preparedState(t)

	_, startErr := action.Start(context.Background(), &state)
	_, stopErr := action.Stop(context.Background(), &state)

	require.Error(t, startErr, "kfake does not implement CreateACLs, so the action must fail")
	require.Error(t, stopErr, "kfake does not implement DeleteACLs, so the action must fail")
}

func TestDenyUser_Start_and_Stop_unknown_cluster(t *testing.T) {
	config.SetClustersForTest(map[string]*config.ClusterConfig{})
	action := &KafkaConsumerDenyAccessAttack{}
	state := KafkaDenyUserState{ClusterName: "missing"}

	_, startErr := action.Start(context.Background(), &state)
	_, stopErr := action.Stop(context.Background(), &state)

	require.ErrorContains(t, startErr, "failed to get cluster config")
	require.ErrorContains(t, stopErr, "failed to get cluster config")
}

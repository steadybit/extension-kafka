// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import (
	"testing"

	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/stretchr/testify/assert"
)

func TestParameterGroups(t *testing.T) {
	recordContent := []string{"recordKey@" + groupRecordContent, "recordHeaders@" + groupRecordContent, "recordValue@" + groupRecordContent}
	produce := func(amount string) []string {
		return append(append([]string{}, recordContent...), amount, "duration", "successRate", "maxConcurrent@"+groupProducerBehavior)
	}
	producerBehavior := []action_kit_api.ParameterGroup{producerBehaviorGroup}

	tests := []struct {
		name    string
		desc    action_kit_api.ActionDescription
		layout  []string
		defined []action_kit_api.ParameterGroup
	}{
		{"brokers", NewBrokersCheckAction().Describe(), []string{"duration", "expectedChanges", "changeCheckMode", "failEarly"}, nil},
		{"consumerGroup", NewConsumerGroupCheckAction().Describe(), []string{"duration", "expectedStateList", "stateCheckMode", "failEarly"}, nil},
		{"partitions", NewPartitionsCheckAction().Describe(), []string{"duration", "expectedChanges", "changeCheckMode", "failEarly"}, nil},
		{"consumerGroupLag", NewConsumerGroupLagCheckAction().Describe(), []string{"duration", "topic", "acceptableLag", "failEarly"}, nil},
		{"produceFixedAmount", NewProduceMessageActionFixedAmount().Describe(), produce("numberOfRecords"), producerBehavior},
		{"producePeriodically", NewProduceMessageActionPeriodically().Describe(), produce("recordsPerSecond"), producerBehavior},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.layout, layout(t, tt.desc))
			assert.Equal(t, tt.defined, definedGroups(tt.desc))
		})
	}
}

func TestRecordValueHasADefault(t *testing.T) {
	// The record value sits in a collapsed group, so a new step must not start with an error hidden inside it.
	for _, desc := range []action_kit_api.ActionDescription{NewProduceMessageActionFixedAmount().Describe(), NewProduceMessageActionPeriodically().Describe()} {
		found := false
		for _, p := range desc.Parameters {
			if p.Name == "recordValue" {
				found = true
				assert.Equal(t, action_kit_api.ActionParameterTypeTextarea, p.Type)
				assert.Equal(t, "test-message", *p.DefaultValue)
			}
		}
		assert.True(t, found, "%s: recordValue parameter", desc.Id)
	}
}

// layout lists the parameters in the order platforms show them, as "name" or "name@group". None of these actions sets
// an order, so the list order decides and a group is shown where its first parameter is.
func layout(t *testing.T, desc action_kit_api.ActionDescription) []string {
	t.Helper()
	var names []string
	for _, p := range desc.Parameters {
		assert.Nil(t, p.Order, "%s: parameter %s has an order, so the list order no longer decides", desc.Id, p.Name)
		if p.Group != nil {
			names = append(names, p.Name+"@"+*p.Group)
		} else {
			names = append(names, p.Name)
		}
		// No advanced parameter is left to the platform's fallback group, and the grouped ones that were advanced stay
		// advanced for platforms without groups.
		if p.Advanced != nil && *p.Advanced {
			assert.NotNil(t, p.Group, "%s: advanced parameter %s has no group", desc.Id, p.Name)
		}
		if p.Name == "failEarly" {
			assert.Nil(t, p.Advanced, "%s: failEarly is shown on every platform", desc.Id)
		}
	}
	return names
}

func definedGroups(desc action_kit_api.ActionDescription) []action_kit_api.ParameterGroup {
	if desc.ParameterGroups == nil {
		return nil
	}
	return *desc.ParameterGroups
}

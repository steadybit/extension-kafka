// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Steadybit GmbH

package extkafka

import "github.com/steadybit/action-kit/go/action_kit_api/v2"

const (
	groupRecordContent    = "Record Content"
	groupProducerBehavior = "Producer Behavior"
)

var (
	producerBehaviorGroup = action_kit_api.ParameterGroup{
		Name:        groupProducerBehavior,
		Description: "Only matters when the broker responds slowly: if all requests are still in flight, the next ones wait and the target rate is not reached.",
	}

	recordKeyContent     = withGroup(recordKey, groupRecordContent)
	recordHeadersContent = withGroup(recordHeaders, groupRecordContent)
	recordValueContent   = withGroup(recordValue, groupRecordContent)
)

func withGroup(p action_kit_api.ActionParameter, group string) action_kit_api.ActionParameter {
	p.Group = &group
	return p
}

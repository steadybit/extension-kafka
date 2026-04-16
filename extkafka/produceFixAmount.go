// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
)

type produceMessageActionFixedAmount struct{}

// Make sure Action implements all required interfaces
var (
	_ action_kit_sdk.Action[KafkaBrokerAttackState]           = (*produceMessageActionFixedAmount)(nil)
	_ action_kit_sdk.ActionWithStatus[KafkaBrokerAttackState] = (*produceMessageActionFixedAmount)(nil)

	_ action_kit_sdk.ActionWithStop[KafkaBrokerAttackState] = (*produceMessageActionFixedAmount)(nil)
)

func NewProduceMessageActionFixedAmount() action_kit_sdk.Action[KafkaBrokerAttackState] {
	return &produceMessageActionFixedAmount{}
}

func (l *produceMessageActionFixedAmount) NewEmptyState() KafkaBrokerAttackState {
	return KafkaBrokerAttackState{}
}

// Describe returns the action description for the platform with all required information.
func (l *produceMessageActionFixedAmount) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.produce-fixed-amount", kafkaTopicTargetId),
		Label:       "Produce (# of Records)",
		Description: "Produce a fixed total number of records to a topic, distributed evenly across the duration. For rate-based production (records/second), use Produce (Records / s) instead.",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        new(kafkaIcon),
		TargetSelection: new(action_kit_api.TargetSelection{
			TargetType: kafkaTopicTargetId,
			SelectionTemplates: new([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "topic name",
					Description: new("Find topic by cluster and name"),
					Query:       "kafka.cluster.name=\"\" AND kafka.topic.name=\"\"",
				},
			}),
		}),
		//Widgets: extutil.Ptr([]action_kit_api.Widget{
		//	action_kit_api.PredefinedWidget{
		//		Type:               action_kit_api.ComSteadybitWidgetPredefined,
		//		PredefinedWidgetId: "com.steadybit.widget.predefined.HttpCheck",
		//	},
		//}),

		// Technology for the targets to appear in
		Technology: new("Kafka"),
		Category:   new("Kafka"),

		// To clarify the purpose of the action:
		//   Check: Will perform checks on the targets
		Kind: action_kit_api.Attack,

		// How the action is controlled over time.
		//   External: The agent takes care and calls stop then the time has passed. Requires a duration parameter. Use this when the duration is known in advance.
		//   Internal: The action has to implement the status endpoint to signal when the action is done. Use this when the duration is not known in advance.
		//   Instantaneous: The action is done immediately. Use this for actions that happen immediately, e.g. a reboot.
		TimeControl: action_kit_api.TimeControlInternal,

		// The parameters for the action
		Parameters: []action_kit_api.ActionParameter{
			//------------------------
			// Request Definition
			//------------------------
			recordKey,
			recordValue,
			recordHeaders,
			{
				Name:  "-",
				Label: "-",
				Type:  action_kit_api.ActionParameterTypeSeparator,
				Order: new(5),
			},
			{
				Name:         "numberOfRecords",
				Label:        "Number of Records.",
				Description:  new("Total number of records to produce across the entire duration. The production rate is this value divided by the duration."),
				Type:         action_kit_api.ActionParameterTypeInteger,
				Required:     new(true),
				DefaultValue: new("1"),
			},
			duration,
			{
				Name:  "-",
				Label: "-",
				Type:  action_kit_api.ActionParameterTypeSeparator,
				Order: new(9),
			},
			successRate,
			//------------------------
			// Additional Settings
			//------------------------

			maxConcurrent,
		},
		Status: new(action_kit_api.MutatingEndpointReferenceWithCallInterval{
			CallInterval: new("1s"),
		}),
		Stop: new(action_kit_api.MutatingEndpointReference{}),
	}
}

func getDelayBetweenRequestsInMsFixedAmount(duration int64, numberOfRequests int64) int64 {
	if duration > 0 && numberOfRequests > 0 {
		return duration / (numberOfRequests)
	} else {
		return 1000 / 1
	}
}

func (l *produceMessageActionFixedAmount) Prepare(_ context.Context, state *KafkaBrokerAttackState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	if extutil.ToInt64(request.Config["duration"]) == 0 {
		return nil, errors.New("duration must be greater than 0")
	}
	state.DelayBetweenRequestsInMS = getDelayBetweenRequestsInMsFixedAmount(extutil.ToInt64(request.Config["duration"]), extutil.ToInt64(request.Config["numberOfRecords"]))

	return prepare(request, state, checkEndedProduceFixedAmount)
}

func checkEndedProduceFixedAmount(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool {
	result := executionRunData.requestCounter.Load() >= state.NumberOfRecords
	return result
}

// Start is called to start the action
// You can mutate the state here.
// You can use the result to return messages/errors/metrics or artifacts
func (l *produceMessageActionFixedAmount) Start(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StartResult, error) {
	start(state)
	return nil, nil
}

// Status is called to get the current status of the action
func (l *produceMessageActionFixedAmount) Status(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StatusResult, error) {
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load execution run data")
		return nil, err
	}

	completed := checkEndedProduceFixedAmount(executionRunData, state)
	if completed {
		stopExecution(executionRunData)
		log.Info().Msg("Action completed")
	}

	latestMetrics := retrieveLatestMetrics(executionRunData.metrics)

	return &action_kit_api.StatusResult{
		Completed: completed,
		Metrics:   new(latestMetrics),
	}, nil
}

func (l *produceMessageActionFixedAmount) Stop(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StopResult, error) {
	return stop(state)
}

func (l *produceMessageActionFixedAmount) getExecutionRunData(executionID uuid.UUID) (*ExecutionRunData, error) {
	return loadExecutionRunData(executionID)
}

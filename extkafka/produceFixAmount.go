/*
* Copyright 2024 steadybit GmbH. All rights reserved.
 */

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
		Description: "Produce a certain amount of kafka records for a given duration",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType: kafkaTopicTargetId,
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "by topic name",
					Description: extutil.Ptr("Find topic by name"),
					Query:       "kafka.topic.name=\"\"",
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
		Technology: extutil.Ptr("Kafka"),
		Category:   extutil.Ptr("Kafka"),

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
				Type:  action_kit_api.Separator,
				Order: extutil.Ptr(5),
			},
			{
				Name:         "numberOfRecords",
				Label:        "Number of Records.",
				Description:  extutil.Ptr("Fixed number of Records, distributed to given duration"),
				Type:         action_kit_api.Integer,
				Required:     extutil.Ptr(true),
				DefaultValue: extutil.Ptr("1"),
			},
			duration,
			{
				Name:  "-",
				Label: "-",
				Type:  action_kit_api.Separator,
				Order: extutil.Ptr(9),
			},
			successRate,
			//------------------------
			// Additional Settings
			//------------------------

			maxConcurrent,
		},
		Status: extutil.Ptr(action_kit_api.MutatingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr("1s"),
		}),
		Stop: extutil.Ptr(action_kit_api.MutatingEndpointReference{}),
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
		stopTickers(executionRunData)
		log.Info().Msg("Action completed")
	}

	latestMetrics := retrieveLatestMetrics(executionRunData.metrics)

	return &action_kit_api.StatusResult{
		Completed: completed,
		Metrics:   extutil.Ptr(latestMetrics),
	}, nil
}

func (l *produceMessageActionFixedAmount) Stop(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StopResult, error) {
	return stop(state)
}

func (l *produceMessageActionFixedAmount) getExecutionRunData(executionID uuid.UUID) (*ExecutionRunData, error) {
	return loadExecutionRunData(executionID)
}

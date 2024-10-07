/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
 */

package extkafka

import (
	"context"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
)

type produceMessageActionPeriodically struct{}

// Make sure Action implements all required interfaces
var (
	_ action_kit_sdk.Action[KafkaBrokerAttackState]           = (*produceMessageActionPeriodically)(nil)
	_ action_kit_sdk.ActionWithStatus[KafkaBrokerAttackState] = (*produceMessageActionPeriodically)(nil)

	_ action_kit_sdk.ActionWithStop[KafkaBrokerAttackState] = (*produceMessageActionPeriodically)(nil)
)

func NewProduceMessageActionPeriodically() action_kit_sdk.Action[KafkaBrokerAttackState] {
	return &produceMessageActionPeriodically{}
}

func (l *produceMessageActionPeriodically) NewEmptyState() KafkaBrokerAttackState {
	return KafkaBrokerAttackState{}
}

// Describe returns the action description for the platform with all required information.
func (l *produceMessageActionPeriodically) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          TargetIDPeriodically,
		Label:       "Produce (Messages / s)",
		Description: "Produce kafka messages periodically (messages / s)",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaMessagePeriodically),
		Widgets: extutil.Ptr([]action_kit_api.Widget{
			action_kit_api.PredefinedWidget{
				Type:               action_kit_api.ComSteadybitWidgetPredefined,
				PredefinedWidgetId: "com.steadybit.widget.predefined.HttpCheck",
			},
		}),

		Technology: extutil.Ptr("Kafka"),

		// To clarify the purpose of the action:
		//   Check: Will perform checks on the targets
		Kind: action_kit_api.LoadTest,

		// How the action is controlled over time.
		//   External: The agent takes care and calls stop then the time has passed. Requires a duration parameter. Use this when the duration is known in advance.
		//   Internal: The action has to implement the status endpoint to signal when the action is done. Use this when the duration is not known in advance.
		//   Instantaneous: The action is done immediately. Use this for actions that happen immediately, e.g. a reboot.
		TimeControl: action_kit_api.TimeControlExternal,

		// The parameters for the action
		Parameters: []action_kit_api.ActionParameter{
			//------------------------
			// Request Definition
			//------------------------
			recordKeyValue,
			recordHeaders,
			{
				Name:  "-",
				Label: "-",
				Type:  action_kit_api.Separator,
				Order: extutil.Ptr(5),
			},
			//------------------------
			// Repetitions
			//------------------------
			repetitionControl,
			{
				Name:         "recordsPerSecond",
				Label:        "Records per second",
				Description:  extutil.Ptr("The number of records per second. Should be between 1 and 10."),
				Type:         action_kit_api.Integer,
				DefaultValue: extutil.Ptr("1"),
				Required:     extutil.Ptr(true),
				Order:        extutil.Ptr(7),
			},
			duration,
			{
				Name:  "-",
				Label: "-",
				Type:  action_kit_api.Separator,
				Order: extutil.Ptr(9),
			},
			//------------------------
			// Result Verification
			//------------------------
			resultVerification,
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

func getDelayBetweenRequestsInMsPeriodically(recordsPerSecond int64) int64 {
	if recordsPerSecond > 0 {
		return 1000 / recordsPerSecond
	} else {
		return 1000 / 1
	}
}

func (l *produceMessageActionPeriodically) Prepare(_ context.Context, state *KafkaBrokerAttackState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	state.DelayBetweenRequestsInMS = getDelayBetweenRequestsInMsPeriodically(extutil.ToInt64(request.Config["recordsPerSecond"]))

	return prepare(request, state, func(executionRunData *ExecutionRunData, state *KafkaBrokerAttackState) bool { return false })
}

// Start is called to start the action
// You can mutate the state here.
// You can use the result to return messages/errors/metrics or artifacts
func (l *produceMessageActionPeriodically) Start(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StartResult, error) {
	start(state)
	return nil, nil
}

// Status is called to get the current status of the action
func (l *produceMessageActionPeriodically) Status(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StatusResult, error) {
	executionRunData, err := loadExecutionRunData(state.ExecutionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load execution run data")
		return nil, err
	}
	latestMetrics := retrieveLatestMetrics(executionRunData.metrics)
	return &action_kit_api.StatusResult{
		Completed: false,
		Metrics:   extutil.Ptr(latestMetrics),
	}, nil
}

func (l *produceMessageActionPeriodically) Stop(_ context.Context, state *KafkaBrokerAttackState) (*action_kit_api.StopResult, error) {
	return stop(state)
}

func (l *produceMessageActionPeriodically) getExecutionRunData(executionID uuid.UUID) (*ExecutionRunData, error) {
	return loadExecutionRunData(executionID)
}

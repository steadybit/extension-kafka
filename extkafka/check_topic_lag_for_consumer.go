// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/action-kit/go/action_kit_sdk"
	"github.com/steadybit/extension-kafka/config"
	extension_kit "github.com/steadybit/extension-kit"
	"github.com/steadybit/extension-kit/extbuild"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
)

type ConsumerGroupLagCheckAction struct{}

type ConsumerGroupLagCheckState struct {
	ConsumerGroupName string
	Topic             string
	End               time.Time
	AcceptableLag     int64
	StateCheckSuccess bool
	StateCheckFailed  bool
	BrokerHosts       []string
	ClusterName       string // Cluster name for multi-cluster support
}

// Make sure action implements all required interfaces
var (
	_ action_kit_sdk.Action[ConsumerGroupLagCheckState]           = (*ConsumerGroupLagCheckAction)(nil)
	_ action_kit_sdk.ActionWithStatus[ConsumerGroupLagCheckState] = (*ConsumerGroupLagCheckAction)(nil)
)

func NewConsumerGroupLagCheckAction() action_kit_sdk.Action[ConsumerGroupLagCheckState] {
	return &ConsumerGroupLagCheckAction{}
}

func (m *ConsumerGroupLagCheckAction) NewEmptyState() ConsumerGroupLagCheckState {
	return ConsumerGroupLagCheckState{}
}

func (m *ConsumerGroupLagCheckAction) Describe() action_kit_api.ActionDescription {
	return action_kit_api.ActionDescription{
		Id:          fmt.Sprintf("%s.check-lag", kafkaConsumerTargetId),
		Label:       "Check Topic Lag",
		Description: "Check the consumer lag for a given topic (lag is calculated by the difference between topic offset and consumer offset)",
		Version:     extbuild.GetSemverVersionStringOrUnknown(),
		Icon:        extutil.Ptr(kafkaIcon),
		TargetSelection: extutil.Ptr(action_kit_api.TargetSelection{
			TargetType:          kafkaConsumerTargetId,
			QuantityRestriction: extutil.Ptr(action_kit_api.QuantityRestrictionAll),
			SelectionTemplates: extutil.Ptr([]action_kit_api.TargetSelectionTemplate{
				{
					Label:       "consumer group name",
					Description: extutil.Ptr("Find consumer group by cluster and name"),
					Query:       "kafka.cluster.name=\"\" AND kafka.consumer-group.name=\"\"",
				},
			}),
		}),
		Technology:  extutil.Ptr("Kafka"),
		Category:    extutil.Ptr("Kafka"),
		Kind:        action_kit_api.Check,
		TimeControl: action_kit_api.TimeControlInternal,
		Parameters: []action_kit_api.ActionParameter{
			{
				Name:         "duration",
				Label:        "Duration",
				Description:  extutil.Ptr(""),
				Type:         action_kit_api.ActionParameterTypeDuration,
				DefaultValue: extutil.Ptr("30s"),
				Required:     extutil.Ptr(true),
			},
			{
				Name:        "topic",
				Label:       "Topic to track lag",
				Description: extutil.Ptr("One topic to track lags"),
				Type:        action_kit_api.ActionParameterTypeString,
				Required:    extutil.Ptr(true),
				Options: extutil.Ptr([]action_kit_api.ParameterOption{
					action_kit_api.ParameterOptionsFromTargetAttribute{
						Attribute: "kafka.consumer-group.topics",
					},
				}),
			},
			{
				Name:         "acceptableLag",
				Label:        "Lag alert threshold",
				Description:  extutil.Ptr("How much lag is acceptable for this topic"),
				Type:         action_kit_api.ActionParameterTypeInteger,
				Required:     extutil.Ptr(true),
				DefaultValue: extutil.Ptr("10"),
			},
		},
		Widgets: extutil.Ptr([]action_kit_api.Widget{
			action_kit_api.LineChartWidget{
				Type:  action_kit_api.ComSteadybitWidgetLineChart,
				Title: "Consumer Group Lag",
				Identity: action_kit_api.LineChartWidgetIdentityConfig{
					MetricName: "kafka_consumer_group_lag",
					From:       "id",
					Mode:       action_kit_api.ComSteadybitWidgetLineChartIdentityModeSelect,
				},
				Grouping: extutil.Ptr(action_kit_api.LineChartWidgetGroupingConfig{
					ShowSummary: extutil.Ptr(true),
					Groups: []action_kit_api.LineChartWidgetGroup{
						{
							Title: "Under Acceptable Lag",
							Color: "success",
							Matcher: action_kit_api.LineChartWidgetGroupMatcherFallback{
								Type: action_kit_api.ComSteadybitWidgetLineChartGroupMatcherFallback,
							},
						},
						{
							Title: "Lag Constraint Violated",
							Color: "warn",
							Matcher: action_kit_api.LineChartWidgetGroupMatcherKeyEqualsValue{
								Type:  action_kit_api.ComSteadybitWidgetLineChartGroupMatcherKeyEqualsValue,
								Key:   "lag_constraints_fulfilled",
								Value: "false",
							},
						},
					},
				}),
				Tooltip: extutil.Ptr(action_kit_api.LineChartWidgetTooltipConfig{
					MetricValueTitle: extutil.Ptr("Lag"),
					AdditionalContent: []action_kit_api.LineChartWidgetTooltipContent{
						{
							From:  "consumer",
							Title: "Consumer",
						},
						{
							From:  "topic",
							Title: "Topic",
						},
					},
				}),
			},
		}),
		Status: extutil.Ptr(action_kit_api.MutatingEndpointReferenceWithCallInterval{
			CallInterval: extutil.Ptr("1s"),
		}),
	}
}

func (m *ConsumerGroupLagCheckAction) Prepare(_ context.Context, state *ConsumerGroupLagCheckState, request action_kit_api.PrepareActionRequestBody) (*action_kit_api.PrepareResult, error) {
	if len(request.Target.Attributes["kafka.consumer-group.name"]) == 0 {
		return nil, fmt.Errorf("the target is missing the kafka.consumer-group.name attribute")
	}
	state.Topic = extutil.ToString(request.Config["topic"])
	state.AcceptableLag = extutil.ToInt64(request.Config["acceptableLag"])
	state.StateCheckFailed = false

	duration := request.Config["duration"].(float64)
	end := time.Now().Add(time.Millisecond * time.Duration(duration))

	// Get cluster name from target
	clusterName := extutil.MustHaveValue(request.Target.Attributes, "kafka.cluster.name")[0]
	clusterConfig, err := config.GetClusterConfig(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	state.ConsumerGroupName = request.Target.Attributes["kafka.consumer-group.name"][0]
	state.ClusterName = clusterName
	state.BrokerHosts = strings.Split(clusterConfig.SeedBrokers, ",")
	state.End = end

	return nil, nil
}

func (m *ConsumerGroupLagCheckAction) Start(_ context.Context, _ *ConsumerGroupLagCheckState) (*action_kit_api.StartResult, error) {
	return nil, nil
}

func (m *ConsumerGroupLagCheckAction) Status(ctx context.Context, state *ConsumerGroupLagCheckState) (*action_kit_api.StatusResult, error) {
	return ConsumerGroupLagCheckStatus(ctx, state)
}

func ConsumerGroupLagCheckStatus(ctx context.Context, state *ConsumerGroupLagCheckState) (*action_kit_api.StatusResult, error) {
	now := time.Now()

	clusterConfig, err := config.GetClusterConfig(state.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	client, err := createNewAdminClientWithConfig(state.BrokerHosts, clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	defer client.Close()

	lags, err := client.Lag(ctx, state.ConsumerGroupName)
	if err != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Failed to retrieve consumer groups from Kafka for name %s. Full response: %v", state.ConsumerGroupName, err), err))
	}

	var groupLag kadm.DescribedGroupLag
	if len(lags.Sorted()) == 0 {
		log.Err(err).Msgf("No lags for consumer group with that name %s.", state.ConsumerGroupName)
	} else if len(lags.Sorted()) > 1 {
		log.Err(err).Msgf("More than 1 lag description for consumer group with that name %s.", state.ConsumerGroupName)
	} else {
		groupLag = lags.Sorted()[0]
	}

	if groupLag.FetchErr != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Error when fetching or describing the consumer group %s: %s", state.ConsumerGroupName, groupLag.FetchErr.Error()), groupLag.FetchErr))
	}
	if groupLag.DescribeErr != nil {
		return nil, extutil.Ptr(extension_kit.ToError(fmt.Sprintf("Error when fetching or describing the consumer group %s: %s", state.ConsumerGroupName, groupLag.DescribeErr.Error()), groupLag.DescribeErr))

	}
	topicLag := groupLag.Lag.TotalByTopic()[state.Topic].Lag

	completed := now.After(state.End)
	var checkError *action_kit_api.ActionKitError
	if topicLag < state.AcceptableLag {
		state.StateCheckSuccess = true
	} else {
		state.StateCheckFailed = true
	}
	if completed && state.StateCheckFailed {
		checkError = extutil.Ptr(action_kit_api.ActionKitError{
			Title: fmt.Sprintf("Consumer Group Lag was higher at least once than acceptable threshold  '%d'.",
				state.AcceptableLag),
			Status: extutil.Ptr(action_kit_api.Failed),
		})
	}

	metrics := []action_kit_api.Metric{
		*toMetric(topicLag, state, now),
	}

	return &action_kit_api.StatusResult{
		Completed: completed,
		Error:     checkError,
		Metrics:   extutil.Ptr(metrics),
	}, nil
}

func toMetric(topicLag int64, stateGroupLag *ConsumerGroupLagCheckState, now time.Time) *action_kit_api.Metric {
	return extutil.Ptr(action_kit_api.Metric{
		Name: extutil.Ptr("kafka_consumer_group_lag"),
		Metric: map[string]string{
			"lag_constraints_fulfilled": strconv.FormatBool(topicLag < stateGroupLag.AcceptableLag),
			"consumer":                  stateGroupLag.ConsumerGroupName,
			"topic":                     stateGroupLag.Topic,
			"id":                        stateGroupLag.ConsumerGroupName + "-" + stateGroupLag.Topic,
		},
		Timestamp: now,
		Value:     float64(topicLag),
	})
}

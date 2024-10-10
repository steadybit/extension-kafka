package extkafka

import (
	"github.com/google/uuid"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kgo"
	"time"
)

const (
	kafkaBrokerTargetId         = "com.steadybit.extension_kafka.broker"
	kafkaConsumerTargetId       = "com.steadybit.extension_kafka.consumer"
	kafkaTopicTargetId          = "com.steadybit.extension_kafka.topic"
	TargetIDProducePeriodically = "com.steadybit.extension_kafka.produce.periodically"
	TargetIDConsumePeriodically = "com.steadybit.extension_kafka.consume.periodically"
	TargetIDProduceFixedAmount  = "com.steadybit.extension_kafka.produce.fixed_amount"
	TargetIDConsumeFixedAmount  = "com.steadybit.extension_kafka.consume.fixed_amount"
)

const (
	kafkaIcon                 = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
	kafkaMessagePeriodically  = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
	kafkaMessageFixedAmount   = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aCBmaWxsPSJjdXJyZW50Q29sb3IiCiAgICBkPSJNMTguNTYsMTQuMjZjLTIuNSwwLTQuNiwyLjEtNC42LDQuNnMyLjEsNC42LDQuNiw0LjYsNC42LTIuMSw0LjYtNC42LTIuMS00LjYtNC42LTQuNlpNMTguNTYsMjIuMjZjLTEuOSwwLTMuNC0xLjUtMy40LTMuNHMxLjUtMy40LDMuNC0zLjQsMy40LDEuNSwzLjQsMy40LTEuNSwzLjQtMy40LDMuNFpNMjAuNDYsMTYuNjZoLS44bC0uMiwxLjJoLTEuMWwuMi0xLjJoLS44bC0uMiwxLjJoLS45di43aC43bC0uMi45aC0uOXYuOGguN2wtLjIsMS4yaC44bC4yLTEuMmgxLjFsLS4yLDEuMmguOHYtLjFsLjItMS4yaC45di0uN2gtLjdsLjItLjloLjl2LS43aC0uN2wuMi0xLjJaTTE5LjA2LDE5LjM2aC0xLjFsLjItLjloMS4xbC0uMi45Wk0xNS4zNiwxNS4wNmMtLjUxLTEuMDUtMS41Ny0xLjc3LTIuODEtMS43Ny0uOTQsMC0xLjc3LjQyLTIuMzQsMS4wOGwtMS42Mi0xLjAzYy4xNy0uNDQuMjctLjkyLjI3LTEuNDMsMC0uNDQtLjA5LS44Ni0uMjItMS4yNWwxLjU1LTEuMDFjLjU3LjY2LDEuNDEsMS4wOSwyLjM1LDEuMDksMS43MywwLDMuMTMtMS40LDMuMTMtMy4xM3MtMS40LTMuMTMtMy4xMy0zLjEzLTMuMTMsMS40LTMuMTMsMy4xM2MwLC4zMy4wNy42NC4xNi45NGwtMS41MS45OGMtLjYtLjgyLTEuNS0xLjM5LTIuNTQtMS41N3YtMS43OGMxLjQxLS4zLDIuNDgtMS41NSwyLjQ4LTMuMDYsMC0xLjczLTEuNC0zLjEzLTMuMTMtMy4xM1MxLjc1LDEuNCwxLjc1LDMuMTNjMCwxLjUxLDEuMDcsMi43NiwyLjQ4LDMuMDZ2MS43OGMtMS45Mi4zLTMuMzksMS45NS0zLjM5LDMuOTVzMS40NywzLjY1LDMuMzksMy45NXYxLjk1Yy0xLjQyLjI5LTIuNSwxLjU1LTIuNSwzLjA2LDAsMS43MywxLjQsMy4xMywzLjEzLDMuMTNzMy4xMy0xLjQsMy4xMy0zLjEzYzAtMS41LTEuMDYtMi43NS0yLjQ2LTMuMDV2LTEuOTZjLjk1LS4xNiwxLjc3LS42NiwyLjM3LTEuMzZsMS42NywxLjA2Yy0uMDguMjgtLjE0LjU2LS4xNC44NiwwLDEuNzMsMS40LDMuMTMsMy4xMywzLjEzLjM3LDAsLjcxLS4wOCwxLjA0LS4xOS0uMDEtLjE1LS4wNS0uMy0uMDUtLjQ2LDAtMS41NS43MS0yLjkxLDEuODEtMy44M1pNMTIuNDcsNmMuODIsMCwxLjQ5LjY3LDEuNDksMS40OXMtLjY3LDEuNDktMS40OSwxLjQ5LTEuNDktLjY3LTEuNDktMS40OS42Ny0xLjQ5LDEuNDktMS40OVpNMy4yOCwzLjFjMC0uODIuNjctMS40OSwxLjQ5LTEuNDlzMS40OS42NywxLjQ5LDEuNDktLjY3LDEuNDktMS40OSwxLjQ5LTEuNDktLjY3LTEuNDktMS40OVpNNi4zMywyMC44OGMwLC44Mi0uNjcsMS40OS0xLjQ5LDEuNDlzLTEuNDktLjY3LTEuNDktMS40OS42Ny0xLjQ5LDEuNDktMS40OSwxLjQ5LjY3LDEuNDksMS40OVpNNC44NSwxNC4xNmMtMS4xOCwwLTIuMTMtLjk1LTIuMTMtMi4xM3MuOTUtMi4xMywyLjEzLTIuMTMsMi4xMy45NSwyLjEzLDIuMTMtLjk1LDIuMTMtMi4xMywyLjEzWk0xMi40NSwxNy44OGMtLjgyLDAtMS40OS0uNjctMS40OS0xLjQ5cy42Ny0xLjQ5LDEuNDktMS40OSwxLjQ5LjY3LDEuNDksMS40OS0uNjcsMS40OS0xLjQ5LDEuNDlaIiAvPgo8L3N2Zz4="
	stateCheckModeAtLeastOnce = "atLeastOnce"
	stateCheckModeAllTheTime  = "allTheTime"
)

type KafkaBrokerAttackState struct {
	Topic                    string
	Partitions               []string
	Offset                   int64
	DelayBetweenRequestsInMS int64
	SuccessRate              int
	Timeout                  time.Time
	MaxConcurrent            int
	RecordKey                string
	RecordValue              string
	RecordPartition          int
	RecordAttrs              uint8
	NumberOfRecords          uint64
	ExecutionID              uuid.UUID
	RecordHeaders            map[string]string
	ConsumerGroup            string
}

var KafkaClient *kgo.Client

var (
	topic = action_kit_api.ActionParameter{
		Name:        "topic",
		Label:       "Topic",
		Description: extutil.Ptr("The Topic to send records to"),
		Type:        action_kit_api.String,
		Order:       extutil.Ptr(1),
		Required:    extutil.Ptr(true),
	}
	recordKey = action_kit_api.ActionParameter{
		Name:        "recordKey",
		Label:       "Record key",
		Description: extutil.Ptr("The Record Key. If none is set, the partition will be choose with round-robin algorithm."),
		Type:        action_kit_api.String,
		Order:       extutil.Ptr(2),
	}
	recordValue = action_kit_api.ActionParameter{
		Name:        "recordValue",
		Label:       "Record value",
		Description: extutil.Ptr("The Record Value."),
		Type:        action_kit_api.String,
		Order:       extutil.Ptr(3),
		Required:    extutil.Ptr(true),
	}
	recordHeaders = action_kit_api.ActionParameter{
		Name:        "recordHeaders",
		Label:       "Record RecordHeaders",
		Description: extutil.Ptr("The Record RecordHeaders."),
		Type:        action_kit_api.KeyValue,
		Order:       extutil.Ptr(4),
	}
	repetitionControl = action_kit_api.ActionParameter{
		Name:  "repetitionControl",
		Label: "Repetition Control",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(5),
	}
	duration = action_kit_api.ActionParameter{
		Name:         "duration",
		Label:        "Duration",
		Description:  extutil.Ptr("In which timeframe should the specified records be produced?"),
		Type:         action_kit_api.Duration,
		DefaultValue: extutil.Ptr("10s"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(6),
	}
	resultVerification = action_kit_api.ActionParameter{
		Name:  "resultVerification",
		Label: "Result Verification",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(7),
	}
	successRate = action_kit_api.ActionParameter{
		Name:         "successRate",
		Label:        "Required Success Rate",
		Description:  extutil.Ptr("How many percent of the records must be at least successful (in terms of the following response verifications) to continue the experiment execution? The result will be evaluated and the end of the given duration."),
		Type:         action_kit_api.Percentage,
		DefaultValue: extutil.Ptr("100"),
		Required:     extutil.Ptr(true),
		Order:        extutil.Ptr(8),
		MinValue:     extutil.Ptr(0),
		MaxValue:     extutil.Ptr(100),
	}
	maxConcurrent = action_kit_api.ActionParameter{
		Name:         "maxConcurrent",
		Label:        "Max concurrent requests",
		Description:  extutil.Ptr("Maximum count on parallel producing requests. (min 1, max 10)"),
		Type:         action_kit_api.Integer,
		DefaultValue: extutil.Ptr("5"),
		Required:     extutil.Ptr(true),
		Advanced:     extutil.Ptr(true),
		Order:        extutil.Ptr(9),
	}
)

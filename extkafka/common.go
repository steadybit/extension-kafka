package extkafka

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/steadybit/action-kit/go/action_kit_api/v2"
	"github.com/steadybit/extension-kafka/config"
	"github.com/steadybit/extension-kit/extutil"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"strconv"
	"strings"
	"time"
)

const (
	kafkaBrokerTargetId         = "com.steadybit.extension_kafka.broker"
	kafkaConsumerTargetId       = "com.steadybit.extension_kafka.consumer"
	kafkaTopicTargetId          = "com.steadybit.extension_kafka.topic"
	TargetIDProducePeriodically = "com.steadybit.extension_kafka.produce.periodically"
	TargetIDProduceFixedAmount  = "com.steadybit.extension_kafka.produce.fixed_amount"
)

const (
	kafkaIcon                 = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
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
	NumberOfRecords          uint64
	ExecutionID              uuid.UUID
	RecordHeaders            map[string]string
	ConsumerGroup            string
}

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
		Label:       "Record Headers",
		Description: extutil.Ptr("The Record Headers."),
		Type:        action_kit_api.KeyValue,
		Order:       extutil.Ptr(4),
	}
	repetitionControl = action_kit_api.ActionParameter{
		Name:  "repetitionControl",
		Label: "Repetition Control",
		Type:  action_kit_api.Header,
		Order: extutil.Ptr(5),
	}
	durationAlter = action_kit_api.ActionParameter{
		Label:        "Duration",
		Description:  extutil.Ptr("The duration of the action. The broker configuration will be reverted at the end of the action."),
		Name:         "duration",
		Type:         action_kit_api.Duration,
		DefaultValue: extutil.Ptr("60s"),
		Required:     extutil.Ptr(true),
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

func createNewClient() (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(strings.Split(config.Config.SeedBrokers, ",")...),
		kgo.ClientID("steadybit"),
	}

	if config.Config.SaslMechanism != "" {
		switch saslMechanism := config.Config.SaslMechanism; saslMechanism {
		case kadm.ScramSha256.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsSha256Mechanism()),
			}...)
		case kadm.ScramSha512.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsSha512Mechanism()),
			}...)
		default:
			opts = append(opts, []kgo.Opt{
				kgo.SASL(plain.Auth{
					User: config.Config.SaslUser,
					Pass: config.Config.SaslPassword,
				}.AsMechanism()),
			}...)
		}
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}

	return client, nil
}

func createNewAdminClient() (*kadm.Client, error) {
	client, err := createNewClient()
	if err != nil {
		return nil, err
	}
	return kadm.NewClient(client), nil
}

func saveConfig(ctx context.Context, configName string, brokerID int32) (string, error) {
	var initialValue string
	adminClient, err := createNewAdminClient()
	if err != nil {
		return "", err
	}
	// Get the initial value
	configs, err := adminClient.DescribeBrokerConfigs(ctx, brokerID)
	if err != nil {
		return "", err
	}
	_, err = configs.On(strconv.FormatInt(int64(brokerID), 10), func(resourceConfig *kadm.ResourceConfig) error {

		for i := range resourceConfig.Configs {
			if resourceConfig.Configs[i].Key == configName {
				initialValue = resourceConfig.Configs[i].MaybeValue()
				// Found!
				break
			}
		}

		return err
	})
	if err != nil {
		return "", err
	}
	if initialValue == "" {
		log.Warn().Msgf("No initial value found for configuration key: "+configName+", for broker node-id: %d", brokerID)
	}

	return initialValue, nil
}

func alterConfig(ctx context.Context, configName string, configValue string, brokerID int32) error {
	adminClient, err := createNewAdminClient()
	if err != nil {
		return err
	}
	defer adminClient.Close()

	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: configName, Value: extutil.Ptr(configValue)}}, brokerID)
	if err != nil {
		return err
	}
	var errs []error
	for _, response := range responses {
		if response.Err != nil {
			detailedError := errors.New(response.Err.Error() + " Response from Broker: " + response.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

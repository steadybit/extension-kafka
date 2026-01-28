// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package extkafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	kafkaBrokerTargetId   = "com.steadybit.extension_kafka.broker"
	kafkaConsumerTargetId = "com.steadybit.extension_kafka.consumer"
	kafkaTopicTargetId    = "com.steadybit.extension_kafka.topic"
)

func init() {
	// Register the cluster name getter with the config package to avoid import cycles
	config.SetClusterNameGetter(GetClusterNameFromConfig)
}

const (
	kafkaIcon                 = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZlcnNpb249IjEuMSIgdmlld0JveD0iMCAwIDI0IDI0Ij4KICA8cGF0aAogICAgZD0iTTE1LjksMTMuMmMtLjksMC0xLjYuNC0yLjIsMWwtMS4zLTFjLjEtLjQuMi0uOC4yLTEuM3MwLS45LS4yLTEuMmwxLjMtLjljLjUuNiwxLjMsMSwyLjEsMSwxLjYsMCwyLjktMS4zLDIuOS0yLjlzLTEuMy0yLjktMi45LTIuOS0yLjksMS4zLTIuOSwyLjksMCwuNi4xLjhsLTEuMy45Yy0uNi0uNy0xLjQtMS4yLTIuMy0xLjN2LTEuNmMxLjMtLjMsMi4zLTEuNCwyLjMtMi44LDAtMS42LTEuMy0yLjktMi45LTIuOXMtMi45LDEuMy0yLjksMi45LDEsMi41LDIuMiwyLjh2MS42Yy0xLjcuMy0zLjEsMS44LTMuMSwzLjZzMS4zLDMuNCwzLjEsMy42djEuN2MtMS4zLjMtMi4zLDEuNC0yLjMsMi44czEuMywyLjksMi45LDIuOSwyLjktMS4zLDIuOS0yLjktMS0yLjUtMi4zLTIuOHYtMS43Yy45LS4xLDEuNy0uNiwyLjMtMS4zbDEuNCwxYzAsLjMtLjEuNS0uMS44LDAsMS42LDEuMywyLjksMi45LDIuOXMyLjktMS4zLDIuOS0yLjktMS4zLTIuOS0yLjktMi45aDBaTTE1LjksNi41Yy44LDAsMS40LjYsMS40LDEuNHMtLjYsMS40LTEuNCwxLjQtMS40LS42LTEuNC0xLjQuNi0xLjQsMS40LTEuNGgwWk03LjUsMy45YzAtLjguNi0xLjQsMS40LTEuNHMxLjQuNiwxLjQsMS40LS42LDEuNC0xLjQsMS40LTEuNC0uNi0xLjQtMS40aDBaTTEwLjMsMjAuMWMwLC44LS42LDEuNC0xLjQsMS40cy0xLjQtLjYtMS40LTEuNC42LTEuNCwxLjQtMS40LDEuNC42LDEuNCwxLjRaTTguOSwxMy45Yy0xLjEsMC0xLjktLjktMS45LTEuOXMuOS0xLjksMS45LTEuOSwxLjkuOSwxLjksMS45LS45LDEuOS0xLjksMS45Wk0xNS45LDE3LjRjLS44LDAtMS40LS42LTEuNC0xLjRzLjYtMS40LDEuNC0xLjQsMS40LjYsMS40LDEuNC0uNiwxLjQtMS40LDEuNFoiCiAgICBmaWxsPSJjdXJyZW50Q29sb3IiIC8+Cjwvc3ZnPg=="
	stateCheckModeAtLeastOnce = "atLeastOnce"
	stateCheckModeAllTheTime  = "allTheTime"
)

type KafkaBrokerAttackState struct {
	Topic                    string
	Partition                int32
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
	BrokerHosts              []string
	ClusterName              string // Cluster name for multi-cluster support
}

type AlterState struct {
	BrokerHosts              []string
	BrokerID                 int32
	InitialBrokerConfigValue int
	TargetBrokerConfigValue  int
	ClusterName              string // Cluster name for multi-cluster support
}

var (
	topic = action_kit_api.ActionParameter{
		Name:        "topic",
		Label:       "Topic",
		Description: extutil.Ptr("The Topic to send records to"),
		Type:        action_kit_api.ActionParameterTypeString,
		Required:    extutil.Ptr(true),
	}
	recordKey = action_kit_api.ActionParameter{
		Name:        "recordKey",
		Label:       "Record key",
		Description: extutil.Ptr("The Record Key. If none is set, the partition will be choose with round-robin algorithm."),
		Type:        action_kit_api.ActionParameterTypeString,
	}
	recordValue = action_kit_api.ActionParameter{
		Name:        "recordValue",
		Label:       "Record value",
		Description: extutil.Ptr("The Record Value."),
		Type:        action_kit_api.ActionParameterTypeString,
		Required:    extutil.Ptr(true),
	}
	recordHeaders = action_kit_api.ActionParameter{
		Name:        "recordHeaders",
		Label:       "Record Headers",
		Description: extutil.Ptr("The Record Headers."),
		Type:        action_kit_api.ActionParameterTypeKeyValue,
	}
	durationAlter = action_kit_api.ActionParameter{
		Label:        "Duration",
		Description:  extutil.Ptr("The duration of the action. The broker configuration will be reverted at the end of the action."),
		Name:         "duration",
		Type:         action_kit_api.ActionParameterTypeDuration,
		DefaultValue: extutil.Ptr("60s"),
		Required:     extutil.Ptr(true),
	}
	duration = action_kit_api.ActionParameter{
		Name:         "duration",
		Label:        "Duration",
		Description:  extutil.Ptr("In which timeframe should the specified records be produced?"),
		Type:         action_kit_api.ActionParameterTypeDuration,
		DefaultValue: extutil.Ptr("10s"),
		Required:     extutil.Ptr(true),
	}
	successRate = action_kit_api.ActionParameter{
		Name:         "successRate",
		Label:        "Required Success Rate",
		Description:  extutil.Ptr("How many percent of the records must be at least successful (in terms of the following response verifications) to continue the experiment execution? The result will be evaluated and the end of the given duration."),
		Type:         action_kit_api.ActionParameterTypePercentage,
		DefaultValue: extutil.Ptr("100"),
		Required:     extutil.Ptr(true),
		MinValue:     extutil.Ptr(0),
		MaxValue:     extutil.Ptr(100),
	}
	maxConcurrent = action_kit_api.ActionParameter{
		Name:         "maxConcurrent",
		Label:        "Max concurrent requests",
		Description:  extutil.Ptr("Maximum count on parallel producing requests. (min 1, max 10)"),
		Type:         action_kit_api.ActionParameterTypeInteger,
		DefaultValue: extutil.Ptr("5"),
		MinValue:     extutil.Ptr(1),
		MaxValue:     extutil.Ptr(10),
		Required:     extutil.Ptr(true),
		Advanced:     extutil.Ptr(true),
	}
)

func createNewClient(brokers []string) (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
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

	if config.Config.KafkaClusterCaFile != "" && config.Config.KafkaClusterCertKeyFile != "" && config.Config.KafkaClusterCertChainFile != "" {
		tlsConfig, err := newTLSConfig(config.Config.KafkaClusterCertChainFile, config.Config.KafkaClusterCertKeyFile, config.Config.KafkaClusterCaFile)
		if err != nil {
			return nil, err
		}

		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	} else if config.Config.KafkaConnectionUseTLS == "true" {
		tlsDialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
		opts = append(opts, kgo.Dialer(tlsDialer.DialContext))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	log.Debug().Msgf("Initiating client with: %v", opts)

	return client, nil
}

func createNewAdminClient(brokers []string) (*kadm.Client, error) {
	client, err := createNewClient(brokers)
	if err != nil {
		return nil, err
	}
	return kadm.NewClient(client), nil
}

// createNewClientWithConfig creates a Kafka client using a specific cluster configuration
func createNewClientWithConfig(brokers []string, clusterConfig *config.ClusterConfig) (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ClientID("steadybit"),
	}

	if clusterConfig.SaslMechanism != "" {
		switch saslMechanism := clusterConfig.SaslMechanism; saslMechanism {
		case kadm.ScramSha256.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: clusterConfig.SaslUser,
					Pass: clusterConfig.SaslPassword,
				}.AsSha256Mechanism()),
			}...)
		case kadm.ScramSha512.String():
			opts = append(opts, []kgo.Opt{
				kgo.SASL(scram.Auth{
					User: clusterConfig.SaslUser,
					Pass: clusterConfig.SaslPassword,
				}.AsSha512Mechanism()),
			}...)
		default:
			opts = append(opts, []kgo.Opt{
				kgo.SASL(plain.Auth{
					User: clusterConfig.SaslUser,
					Pass: clusterConfig.SaslPassword,
				}.AsMechanism()),
			}...)
		}
	}

	if clusterConfig.KafkaClusterCaFile != "" && clusterConfig.KafkaClusterCertKeyFile != "" && clusterConfig.KafkaClusterCertChainFile != "" {
		tlsConfig, err := newTLSConfig(clusterConfig.KafkaClusterCertChainFile, clusterConfig.KafkaClusterCertKeyFile, clusterConfig.KafkaClusterCaFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	} else if clusterConfig.KafkaConnectionUseTLS == "true" {
		tlsDialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
		opts = append(opts, kgo.Dialer(tlsDialer.DialContext))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kafka client: %s", err.Error())
	}
	log.Debug().Msgf("Initiating client with: %v", opts)

	return client, nil
}

// createNewAdminClientWithConfig creates a Kafka admin client using a specific cluster configuration
func createNewAdminClientWithConfig(brokers []string, clusterConfig *config.ClusterConfig) (*kadm.Client, error) {
	client, err := createNewClientWithConfig(brokers, clusterConfig)
	if err != nil {
		return nil, err
	}
	return kadm.NewClient(client), nil
}

// createNewClientForCluster creates a Kafka client for a specific cluster by name
func createNewClientForCluster(clusterName string) (*kgo.Client, error) {
	clusterConfig, err := config.GetClusterConfig(clusterName)
	if err != nil {
		return nil, err
	}
	brokers := extutil.ToStringArray(clusterConfig.SeedBrokers)
	return createNewClientWithConfig(brokers, clusterConfig)
}

// createNewAdminClientForCluster creates a Kafka admin client for a specific cluster by name
func createNewAdminClientForCluster(clusterName string) (*kadm.Client, error) {
	client, err := createNewClientForCluster(clusterName)
	if err != nil {
		return nil, err
	}
	return kadm.NewClient(client), nil
}

// GetClusterNameFromConfig connects to a cluster and retrieves its name from Kafka metadata
// This function is exported so it can be called from the config package
func GetClusterNameFromConfig(clusterConfig *config.ClusterConfig) (string, error) {
	brokers := strings.Split(clusterConfig.SeedBrokers, ",")
	client, err := createNewClientWithConfig(brokers, clusterConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	adminClient := kadm.NewClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metadata, err := adminClient.BrokerMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get broker metadata: %w", err)
	}

	if metadata.Cluster == "" {
		return "", fmt.Errorf("cluster name is empty in metadata")
	}

	return metadata.Cluster, nil
}

// RetryPendingClusters attempts to resolve cluster names for any clusters that were
// unreachable at startup. Successfully resolved clusters are registered in the active config.
func RetryPendingClusters() {
	pending := config.GetPendingClusters()
	if len(pending) == 0 {
		return
	}

	log.Info().Msgf("Retrying %d pending cluster(s)...", len(pending))
	for _, p := range pending {
		clusterID, err := GetClusterNameFromConfig(p.Config)
		if err != nil {
			log.Warn().Err(err).Msgf("Still unable to resolve cluster ID for CLUSTER_%d, will retry on next discovery", p.Index)
			continue
		}

		if err := config.RegisterCluster(clusterID, p.Config, p.Index); err != nil {
			log.Warn().Err(err).Msgf("Failed to register CLUSTER_%d", p.Index)
			continue
		}

		log.Info().Msgf("Successfully registered previously pending cluster: %d (id: %s, from CLUSTER_%d_*)", p.Index, clusterID, p.Index)
	}
}

func describeConfigInt(ctx context.Context, brokers []string, configName string, brokerID int32) (int, error) {
	value, err := describeConfigStr(ctx, brokers, configName, brokerID)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(value)
}

func describeConfigIntWithConfig(ctx context.Context, brokers []string, configName string, brokerID int32, clusterConfig *config.ClusterConfig) (int, error) {
	value, err := describeConfigStrWithConfig(ctx, brokers, configName, brokerID, clusterConfig)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(value)
}

func describeConfigStr(ctx context.Context, brokers []string, configName string, brokerID int32) (string, error) {
	adminClient, err := createNewAdminClient(brokers)
	if err != nil {
		return "", err
	}
	return describeConfigOf(ctx, adminClient, configName, brokerID)
}

func describeConfigStrWithConfig(ctx context.Context, brokers []string, configName string, brokerID int32, clusterConfig *config.ClusterConfig) (string, error) {
	adminClient, err := createNewAdminClientWithConfig(brokers, clusterConfig)
	if err != nil {
		return "", err
	}
	return describeConfigOf(ctx, adminClient, configName, brokerID)
}

func describeConfigOf(ctx context.Context, adminClient *kadm.Client, configName string, brokerID int32) (configValue string, err error) {
	configs, err := adminClient.DescribeBrokerConfigs(ctx, brokerID)
	if err != nil {
		return "", err
	}

	_, err = configs.On(strconv.FormatInt(int64(brokerID), 10), func(resourceConfig *kadm.ResourceConfig) error {
		for i := range resourceConfig.Configs {
			if resourceConfig.Configs[i].Key == configName {
				configValue = resourceConfig.Configs[i].MaybeValue()
				return nil
			}
		}

		var values []string
		for _, c := range resourceConfig.Configs {
			values = append(values, fmt.Sprintf("%s: %s", c.Key, *c.Value))
		}
		log.Debug().Strs("configs", values).Msgf("Configuration property %s could not be found", configName)

		return nil
	})
	if err != nil {
		return "", err
	}

	if configValue == "" {
		log.Warn().Msgf("No value found for configuration key: %s, for broker node-id: %d", configName, brokerID)
	} else {
		log.Debug().Msgf("Configuration value for key %s: %s, for broker node-id: %d", configName, configValue, brokerID)
	}

	return configValue, nil
}

func alterConfigInt(ctx context.Context, brokers []string, configName string, configValue int, brokerID int32) error {
	return alterConfigStr(ctx, brokers, configName, strconv.Itoa(configValue), brokerID)
}

func alterConfigIntWithConfig(ctx context.Context, brokers []string, configName string, configValue int, brokerID int32, clusterConfig *config.ClusterConfig) error {
	return alterConfigStrWithConfig(ctx, brokers, configName, strconv.Itoa(configValue), brokerID, clusterConfig)
}

func alterConfigStr(ctx context.Context, brokers []string, configName string, configValue string, brokerID int32) error {
	adminClient, err := createNewAdminClient(brokers)
	if err != nil {
		return err
	}
	defer adminClient.Close()

	op := kadm.SetConfig
	if configValue == "" {
		op = kadm.DeleteConfig
	}
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: configName, Value: extutil.Ptr(configValue), Op: op}}, brokerID)
	if err != nil {
		return err
	}
	var errs []error
	for _, response := range responses {
		if response.Err != nil {
			detailedError := fmt.Errorf("%w Response from Broker: %s", response.Err, response.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	// Changes may take time to be applied, wait accordingly
	now := time.Now()
	for {
		if time.Since(now).Seconds() > 5 {
			return fmt.Errorf("configuration change of %s to %s was not applied in time", configName, configValue)
		}
		value, err := describeConfigOf(ctx, adminClient, configName, brokerID)
		if err != nil {
			return err
		}
		if value == configValue {
			return nil
		}
		log.Debug().Msgf("Configuration change of %s to %s was not applied yet, waiting", configName, configValue)
	}
}

func alterConfigStrWithConfig(ctx context.Context, brokers []string, configName string, configValue string, brokerID int32, clusterConfig *config.ClusterConfig) error {
	adminClient, err := createNewAdminClientWithConfig(brokers, clusterConfig)
	if err != nil {
		return err
	}
	defer adminClient.Close()

	op := kadm.SetConfig
	if configValue == "" {
		op = kadm.DeleteConfig
	}
	responses, err := adminClient.AlterBrokerConfigs(ctx, []kadm.AlterConfig{{Name: configName, Value: extutil.Ptr(configValue), Op: op}}, brokerID)
	if err != nil {
		return err
	}
	var errs []error
	for _, response := range responses {
		if response.Err != nil {
			detailedError := fmt.Errorf("%w Response from Broker: %s", response.Err, response.ErrMessage)
			errs = append(errs, detailedError)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	// Changes may take time to be applied, wait accordingly
	now := time.Now()
	for {
		if time.Since(now).Seconds() > 5 {
			return fmt.Errorf("configuration change of %s to %s was not applied in time", configName, configValue)
		}
		value, err := describeConfigOf(ctx, adminClient, configName, brokerID)
		if err != nil {
			return err
		}
		if value == configValue {
			return nil
		}
		log.Debug().Msgf("Configuration change of %s to %s was not applied yet, waiting", configName, configValue)
	}
}

func adjustThreads(ctx context.Context, hosts []string, configName string, targetValue int, brokerId int32) error {
	currentValue, err := describeConfigInt(ctx, hosts, configName, brokerId)
	if err != nil {
		return err
	}

	// As kafka does not allow to more than double or halve the number of threads, we use an iterative approach to get to that value
	for currentValue != targetValue {
		nextValue := max(min(targetValue, currentValue*2), currentValue/2)

		if err := alterConfigInt(ctx, hosts, configName, nextValue, brokerId); err != nil {
			return err
		} else {
			currentValue = nextValue
		}
	}
	return nil
}

func adjustThreadsWithConfig(ctx context.Context, hosts []string, configName string, targetValue int, brokerId int32, clusterConfig *config.ClusterConfig) error {
	currentValue, err := describeConfigIntWithConfig(ctx, hosts, configName, brokerId, clusterConfig)
	if err != nil {
		return err
	}

	// As kafka does not allow to more than double or halve the number of threads, we use an iterative approach to get to that value
	for currentValue != targetValue {
		nextValue := max(min(targetValue, currentValue*2), currentValue/2)

		if err := alterConfigIntWithConfig(ctx, hosts, configName, nextValue, brokerId, clusterConfig); err != nil {
			return err
		} else {
			currentValue = nextValue
		}
	}
	return nil
}

func newTLSConfig(clientCertFile, clientKeyFile, caCertFile string) (*tls.Config, error) {
	tlsConfig := tls.Config{}

	// Load client cert
	cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return &tlsConfig, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	// Load CA cert
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return &tlsConfig, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool

	return &tlsConfig, err
}

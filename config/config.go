// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2025 Steadybit GmbH

package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

// ClusterConfig represents the configuration for a single Kafka cluster
type ClusterConfig struct {
	SeedBrokers               string
	SaslMechanism             string
	SaslUser                  string
	SaslPassword              string
	KafkaConnectionUseTLS     string
	KafkaClusterCertChainFile string
	KafkaClusterCertKeyFile   string
	KafkaClusterCaFile        string
}

// Specification is the configuration specification for the extension. Configuration values can be applied
// through environment variables. Learn more through the documentation of the envconfig package.
// https://github.com/kelseyhightower/envconfig
type Specification struct {
	SeedBrokers                               string   `json:"seedBrokers" required:"false" split_words:"true"`
	SaslMechanism                             string   `json:"saslMechanism" required:"false" split_words:"true"`
	SaslUser                                  string   `json:"saslUser" required:"false" split_words:"true"`
	SaslPassword                              string   `json:"saslPassword" required:"false" split_words:"true"`
	KafkaConnectionUseTLS                     string   `json:"kafkaConnectionUseTLS" required:"false" split_words:"true"`
	KafkaClusterCertChainFile                 string   `json:"kafkaClusterCertChainFile" required:"false" split_words:"true"`
	KafkaClusterCertKeyFile                   string   `json:"kafkaClusterCertKeyFile" required:"false" split_words:"true"`
	KafkaClusterCaFile                        string   `json:"kafkaClusterCaFile" required:"false" split_words:"true"`
	DiscoveryIntervalConsumerGroup            int      `json:"discoveryIntervalKafkaConsumerGroup" split_words:"true" required:"false" default:"30"`
	DiscoveryIntervalKafkaBroker              int      `json:"discoveryIntervalKafkaBroker" split_words:"true" required:"false" default:"30"`
	DiscoveryIntervalKafkaTopic               int      `json:"discoveryIntervalKafkaTopic" split_words:"true" required:"false" default:"30"`
	DiscoveryAttributesExcludesBrokers        []string `json:"discoveryAttributesExcludesBrokers" split_words:"true" required:"false"`
	DiscoveryAttributesExcludesTopics         []string `json:"discoveryAttributesExcludesTopics" split_words:"true" required:"false"`
	DiscoveryAttributesExcludesConsumerGroups []string `json:"discoveryAttributesExcludesConsumerGroups" split_words:"true" required:"false"`

	// Clusters is a map of cluster name to cluster configuration. Populated by parseClusterConfigs().
	Clusters map[string]*ClusterConfig `json:"clusters" ignored:"true"`
}

var (
	Config Specification
	// clustersMutex protects concurrent access to Config.Clusters
	clustersMutex sync.RWMutex
)

func ParseConfiguration() {
	err := envconfig.Process("steadybit_extension", &Config)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to parse configuration from environment.")
	}

	// Parse indexed cluster configs
	clusters := parseClusterConfigs()

	// Backward compatibility: if no indexed clusters found, use legacy config
	if len(clusters) == 0 {
		if Config.SeedBrokers == "" {
			log.Fatal().Msg("No cluster configuration found. Please set either STEADYBIT_EXTENSION_CLUSTER_0_SEED_BROKERS or STEADYBIT_EXTENSION_SEED_BROKERS")
		}

		log.Info().Msg("Using legacy single-cluster configuration")
		legacyCluster := &ClusterConfig{
			SeedBrokers:               Config.SeedBrokers,
			SaslMechanism:             Config.SaslMechanism,
			SaslUser:                  Config.SaslUser,
			SaslPassword:              Config.SaslPassword,
			KafkaConnectionUseTLS:     Config.KafkaConnectionUseTLS,
			KafkaClusterCertChainFile: Config.KafkaClusterCertChainFile,
			KafkaClusterCertKeyFile:   Config.KafkaClusterCertKeyFile,
			KafkaClusterCaFile:        Config.KafkaClusterCaFile,
		}

		clusterName, err := getClusterName(legacyCluster)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get cluster name from legacy config")
		}

		clusters = make(map[string]*ClusterConfig)
		clusters[clusterName] = legacyCluster
		log.Info().Msgf("Registered legacy cluster: %s", clusterName)
	}

	// Set clusters with write lock
	clustersMutex.Lock()
	Config.Clusters = clusters
	clustersMutex.Unlock()
}

func ValidateConfiguration() {
	// You may optionally validate the configuration here.
}

// parseClusterConfigs parses indexed cluster environment variables (CLUSTER_0_*, CLUSTER_1_*, etc.)
func parseClusterConfigs() map[string]*ClusterConfig {
	clusters := make(map[string]*ClusterConfig)
	clusterIndexMap := make(map[string]int) // Track which index each cluster name came from

	for i := 0; ; i++ {
		prefix := fmt.Sprintf("STEADYBIT_EXTENSION_CLUSTER_%d_", i)
		seedBrokers := os.Getenv(prefix + "SEED_BROKERS")

		if seedBrokers == "" {
			break // No more clusters
		}

		clusterConfig := &ClusterConfig{
			SeedBrokers:               seedBrokers,
			SaslMechanism:             os.Getenv(prefix + "SASL_MECHANISM"),
			SaslUser:                  os.Getenv(prefix + "SASL_USER"),
			SaslPassword:              os.Getenv(prefix + "SASL_PASSWORD"),
			KafkaConnectionUseTLS:     os.Getenv(prefix + "KAFKA_CONNECTION_USE_TLS"),
			KafkaClusterCertChainFile: os.Getenv(prefix + "KAFKA_CLUSTER_CERT_CHAIN_FILE"),
			KafkaClusterCertKeyFile:   os.Getenv(prefix + "KAFKA_CLUSTER_CERT_KEY_FILE"),
			KafkaClusterCaFile:        os.Getenv(prefix + "KAFKA_CLUSTER_CA_FILE"),
		}

		// Get cluster name by connecting to this cluster
		clusterName, err := getClusterName(clusterConfig)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to get cluster name for CLUSTER_%d, skipping", i)
			continue
		}

		// Check for duplicates
		if existingIndex, exists := clusterIndexMap[clusterName]; exists {
			log.Fatal().Msgf("Duplicate cluster name '%s' detected (from CLUSTER_%d and CLUSTER_%d). Each cluster must have a unique name.", clusterName, existingIndex, i)
		}

		clusters[clusterName] = clusterConfig
		clusterIndexMap[clusterName] = i
		log.Info().Msgf("Registered cluster: %s (from CLUSTER_%d_*)", clusterName, i)
	}

	return clusters
}

// getClusterName connects to a cluster and retrieves its name from Kafka metadata
// Note: This will be set by the extkafka package during initialization to avoid import cycles
var getClusterName func(*ClusterConfig) (string, error)

// SetClusterNameGetter sets the function used to retrieve cluster names
// This is called from extkafka package to avoid import cycles
func SetClusterNameGetter(getter func(*ClusterConfig) (string, error)) {
	getClusterName = getter
}

// GetClusterConfig looks up cluster configuration by cluster name
func GetClusterConfig(clusterName string) (*ClusterConfig, error) {
	clustersMutex.RLock()
	cluster, ok := Config.Clusters[clusterName]
	clustersMutex.RUnlock()

	if !ok {
		return nil, fmt.Errorf("cluster configuration not found for cluster: %s", clusterName)
	}
	return cluster, nil
}

// GetAllClusterConfigs returns a copy of all configured clusters
func GetAllClusterConfigs() map[string]*ClusterConfig {
	clustersMutex.RLock()
	defer clustersMutex.RUnlock()

	// Return a shallow copy to avoid holding the lock while caller uses the map
	clusters := make(map[string]*ClusterConfig, len(Config.Clusters))
	for k, v := range Config.Clusters {
		clusters[k] = v
	}
	return clusters
}

// SetClustersForTest is a test helper that safely sets cluster configurations with proper locking.
// This should only be used in tests to avoid race conditions.
func SetClustersForTest(clusters map[string]*ClusterConfig) {
	clustersMutex.Lock()
	defer clustersMutex.Unlock()
	Config.Clusters = clusters
}

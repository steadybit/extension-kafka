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
	ClusterID                 string // Kafka internal cluster ID from metadata
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

// PendingCluster represents a cluster configuration that failed name resolution at startup
type PendingCluster struct {
	Index  int
	Config *ClusterConfig
}

var (
	Config Specification
	// clustersMutex protects concurrent access to Config.Clusters
	clustersMutex sync.RWMutex
	// pendingClusters holds cluster configs that failed name resolution and need retry
	pendingClusters []PendingCluster
	pendingMutex    sync.Mutex
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

		clusterID, err := getClusterName(legacyCluster)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get cluster ID from legacy config")
		}

		legacyCluster.ClusterID = clusterID
		// For legacy single-cluster, use the Kafka internal ID as the cluster name
		clusters = make(map[string]*ClusterConfig)
		clusters[clusterID] = legacyCluster
		log.Info().Msgf("Registered legacy cluster: %s", clusterID)
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

		// Get cluster ID by connecting to this cluster
		clusterID, err := getClusterName(clusterConfig)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to get cluster ID for CLUSTER_%d, will retry during discovery", i)
			pendingMutex.Lock()
			pendingClusters = append(pendingClusters, PendingCluster{Index: i, Config: clusterConfig})
			pendingMutex.Unlock()
			continue
		}

		clusterConfig.ClusterID = clusterID
		clusterName := fmt.Sprintf("%d", i)
		clusters[clusterName] = clusterConfig
		clusterIndexMap[clusterName] = i
		log.Info().Msgf("Registered cluster: %s (id: %s, from CLUSTER_%d_*)", clusterName, clusterID, i)
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

// GetPendingClusters returns a copy of the pending clusters list
func GetPendingClusters() []PendingCluster {
	pendingMutex.Lock()
	defer pendingMutex.Unlock()
	result := make([]PendingCluster, len(pendingClusters))
	copy(result, pendingClusters)
	return result
}

// RegisterCluster adds a resolved cluster to the active clusters map and removes it from pending.
// The clusterID is the Kafka internal cluster ID from metadata. The map key is the index string.
func RegisterCluster(clusterID string, clusterConfig *ClusterConfig, index int) error {
	clustersMutex.Lock()
	defer clustersMutex.Unlock()

	clusterConfig.ClusterID = clusterID
	clusterName := fmt.Sprintf("%d", index)
	if _, exists := Config.Clusters[clusterName]; exists {
		return fmt.Errorf("duplicate cluster name '%s' detected (from CLUSTER_%d)", clusterName, index)
	}

	Config.Clusters[clusterName] = clusterConfig

	// Remove from pending
	pendingMutex.Lock()
	defer pendingMutex.Unlock()
	for i, p := range pendingClusters {
		if p.Index == index {
			pendingClusters = append(pendingClusters[:i], pendingClusters[i+1:]...)
			break
		}
	}

	return nil
}

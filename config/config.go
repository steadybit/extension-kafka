/*
 * Copyright 2024 steadybit GmbH. All rights reserved.
 */

package config

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

// Specification is the configuration specification for the extension. Configuration values can be applied
// through environment variables. Learn more through the documentation of the envconfig package.
// https://github.com/kelseyhightower/envconfig
type Specification struct {
	// variable STEADYBIT_EXTENSION_SEED_BROKERS="localhost:9092"
	SeedBrokers string `json:"seedBrokers" required:"true" split_words:"true"`
	// variable STEADYBIT_EXTENSION_SASL_MECHANISM="PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512."
	SaslMechanism string `json:"saslMechanism" required:"false" split_words:"true"`
	// variable STEADYBIT_EXTENSION_SASL_USER="USER"
	SaslUser string `json:"saslUser" required:"false" split_words:"true"`
	// variable STEADYBIT_EXTENSION_SASL_PASSWORD="PASSWORD"
	SaslPassword string `json:"saslPassword" required:"false" split_words:"true"`
	// variable STEADYBIT_EXTENSION_CERT_CHAIN_FILE="/path/to/certfile"
	CertChainFile string `json:"certChainFile" required:"false" split_words:"true"`
	// variable STEADYBIT_EXTENSION_CERT_KEY_FILE="/path/to/keyfile"
	CertKeyFile string `json:"certKeyFile" required:"false" split_words:"true"`
	// variable STEADYBIT_EXTENSION_CA_FILE="/path/to/cafile"
	CaFile                                    string   `json:"caFile" required:"false" split_words:"true"`
	DiscoveryIntervalConsumerGroup            int      `json:"discoveryIntervalKafkaConsumerGroup" split_words:"true" required:"false" default:"30"`
	DiscoveryIntervalKafkaBroker              int      `json:"discoveryIntervalKafkaBroker" split_words:"true" required:"false" default:"30"`
	DiscoveryIntervalKafkaTopic               int      `json:"discoveryIntervalKafkaTopic" split_words:"true" required:"false" default:"30"`
	DiscoveryAttributesExcludesBrokers        []string `json:"discoveryAttributesExcludesBrokers" split_words:"true" required:"false"`
	DiscoveryAttributesExcludesTopics         []string `json:"discoveryAttributesExcludesTopics" split_words:"true" required:"false"`
	DiscoveryAttributesExcludesConsumerGroups []string `json:"discoveryAttributesExcludesConsumerGroups" split_words:"true" required:"false"`
}

var (
	Config Specification
)

func ParseConfiguration() {
	err := envconfig.Process("steadybit_extension", &Config)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to parse configuration from environment.")
	}
}

func ValidateConfiguration() {
	// You may optionally validate the configuration here.
}

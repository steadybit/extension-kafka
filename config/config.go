/*
 * Copyright 2023 steadybit GmbH. All rights reserved.
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
	// This is just a sample configuration value. You can remove it. To be set, you would set the environment
	// variable STEADYBIT_EXTENSION_ROBOT_NAMES="R2-D2,C-3PO".
	RobotNames []string `json:"robotNames" split_words:"true" required:"true" default:"Bender,Terminator,R2-D2"`
	// variable STEADYBIT_EXTENSION_DISCOVERY_ATTRIBUTES_EXCLUDES_ROBOT="robot.label.a,robot.tags.*".
	DiscoveryAttributesExcludesRobot []string `json:"discoveryAttributesExcludesRobot" split_words:"true" required:"false"`
	// variable STEADYBIT_EXTENSION_ACTIVE_ADVICE_LIST="robot_maintenance". To activate all advice, set the value to "*". To exclude some advice, set the value to e.g. "com.steadybit.extension_scaffold.advice.robot-maintenance".
	ActiveAdviceList []string `required:"false" split_words:"true" default:"*"`
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

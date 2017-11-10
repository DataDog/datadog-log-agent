// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/viper"
)

// HOSTNAME is a global to be used as a fallback
var HOSTNAME string

// MainConfig is the name of the main config file, while we haven't merged in dd agent
const MainConfig = "datadog"
const DeprecatedConfig = "logs-agent"

// LogsAgent is the global configuration object
var LogsAgent = viper.New()

// BuildLogsAgentConfig initializes the LogsAgent config and sets default values
func BuildLogsAgentConfig(ddconfigPath, ddconfdPath string) error {
	return buildMainConfig(LogsAgent, ddconfigPath, ddconfdPath)
}

// For legacy reasons, we support setting the configuration in a logs-agent.yaml file
// We will drop this behavior when we stop agent5 support
func isRunningOnAgent5(ddconfigPath string) bool {
	return strings.HasSuffix(ddconfigPath, "datadog.conf")
}

func buildMainConfig(config *viper.Viper, ddconfigPath, ddconfdPath string) error {

	isAgent5 := isRunningOnAgent5(ddconfigPath)

	if isAgent5 {
		// for agent5, we can't parse datadog.conf with viper
		// thus we need a new configuration file
		config.SetConfigFile(filepath.Join(ddconfdPath, "logs-agent.yaml"))
	} else {
		config.SetConfigFile(ddconfigPath)
	}

	err := config.ReadInConfig()
	if err != nil {
		return err
	}

	config.SetDefault("logset", "")
	config.SetDefault("log_dd_url", "intake.logs.datadoghq.com")
	config.SetDefault("log_dd_port", 10516)
	config.SetDefault("skip_ssl_validation", false)
	config.SetDefault("run_path", "/opt/datadog-agent/run")

	if isAgent5 {
		// for agent5, we don't want people to have to set log_enabled in the config
		config.SetDefault("log_enabled", true)
	} else {
		config.SetDefault("log_enabled", false)
	}

	// For hostname, use value from config if set and non empty,
	// or fallback on agent6's logic
	if config.GetString("hostname") == "" {
		hostname, err := util.GetHostname()
		if err != nil {
			log.Println(err)
			hostname = "unknown"
		}
		config.Set("hostname", hostname)
	}

	// save the hostname to a global
	HOSTNAME = hostname

	err = BuildLogsAgentIntegrationsConfigs(ddconfdPath)
	if err != nil {
		return err
	}
	return nil
}

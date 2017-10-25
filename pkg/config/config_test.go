// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const testsPath = "tests"

func TestBuildConfigWithCompleteFile(t *testing.T) {
	var testConfig = viper.New()
	ddconfigPath := filepath.Join(testsPath, "complete", "datadog.yaml")
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	assert.Equal(t, false, isRunningOnAgent5(ddconfigPath))
	buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.Equal(t, "helloworld", testConfig.GetString("api_key"))
	assert.Equal(t, "my.host", testConfig.GetString("hostname"))
	assert.Equal(t, "playground", testConfig.GetString("logset"))
	assert.Equal(t, "my.url", testConfig.GetString("log_dd_url"))
	assert.Equal(t, 10516, testConfig.GetInt("log_dd_port"))
	assert.Equal(t, true, testConfig.GetBool("skip_ssl_validation"))
	assert.Equal(t, true, testConfig.GetBool("log_enabled"))
}

func TestBuildConfigWithCompleteFileAgent5(t *testing.T) {
	var testConfig = viper.New()
	ddconfigPath := filepath.Join(testsPath, "complete5", "datadog.conf")
	ddconfdPath := filepath.Join(testsPath, "complete5", "conf.d")
	assert.Equal(t, true, isRunningOnAgent5(ddconfigPath))
	buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.Equal(t, "helloworld", testConfig.GetString("api_key"))
	assert.Equal(t, "my.host", testConfig.GetString("hostname"))
	assert.Equal(t, "playground", testConfig.GetString("logset"))
	assert.Equal(t, "my.url", testConfig.GetString("log_dd_url"))
	assert.Equal(t, 10516, testConfig.GetInt("log_dd_port"))
	assert.Equal(t, true, testConfig.GetBool("skip_ssl_validation"))
	assert.Equal(t, true, testConfig.GetBool("log_enabled"))
}

func TestBuildConfigWithIncompleteFile(t *testing.T) {
	var testConfig = viper.New()
	ddconfigPath := filepath.Join(testsPath, "incomplete", "datadog.yaml")
	ddconfdPath := filepath.Join(testsPath, "incomplete", "conf.d")
	buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.Equal(t, "", testConfig.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", testConfig.GetString("log_dd_url"))
	assert.Equal(t, 10516, testConfig.GetInt("log_dd_port"))
	assert.Equal(t, false, testConfig.GetBool("skip_ssl_validation"))
	assert.Equal(t, false, testConfig.GetBool("log_enabled"))
}

func TestComputeConfigWithMisconfiguredFile(t *testing.T) {
	var testConfig = viper.New()
	var ddconfigPath, ddconfdPath string
	var err error
	ddconfigPath = filepath.Join(testsPath, "misconfigured_1", "datadog.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_2", "datadog.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_3", "datadog.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_4", "datadog.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_5", "datadog.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_5", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)
}

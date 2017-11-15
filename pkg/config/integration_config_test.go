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

func TestAvailableIntegrationConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	assert.Equal(t, []string{"integration", "integration2", "integration3"}, availableIntegrationConfigs(ddconfdPath))
	ddconfdPath = filepath.Join(testsPath, "complete5", "conf.d")
	assert.Equal(t, []string{"integration"}, availableIntegrationConfigs(ddconfdPath))
}

func TestBuildLogsAgentIntegrationsConfigs(t *testing.T) {
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	var testConfig = viper.New()
	buildLogsAgentIntegrationsConfig(testConfig, ddconfdPath)

	rules := getLogsSources(testConfig)
	assert.Equal(t, 3, len(rules))
	assert.Equal(t, "file", rules[0].Type)
	assert.Equal(t, "/var/log/access.log", rules[0].Path)
	assert.Equal(t, "nginx", rules[0].Service)
	assert.Equal(t, "nginx", rules[0].Source)
	assert.Equal(t, "http_access", rules[0].SourceCategory)
	assert.Equal(t, "", rules[0].Logset)
	assert.Equal(t, "env:prod", rules[0].Tags)
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"env:prod\"]", string(rules[0].TagsPayload))

	assert.Equal(t, "tcp", rules[1].Type)
	assert.Equal(t, 10514, rules[1].Port)
	assert.Equal(t, "devteam", rules[1].Logset)
	assert.Equal(t, "", rules[1].Service)
	assert.Equal(t, "", rules[1].Source)
	assert.Equal(t, 0, len(rules[1].Tags))

	assert.Equal(t, "docker", rules[2].Type)
	assert.Equal(t, "test", rules[2].Image)

	// processing
	assert.Equal(t, 0, len(rules[0].ProcessingRules))
	assert.Equal(t, 1, len(rules[1].ProcessingRules))
	pRule := rules[1].ProcessingRules[0]
	assert.Equal(t, "mask_sequences", pRule.Type)
	assert.Equal(t, "mocked_mask_rule", pRule.Name)
	assert.Equal(t, "[mocked]", pRule.ReplacePlaceholder)
	assert.Equal(t, []byte("[mocked]"), pRule.ReplacePlaceholderBytes)
	assert.Equal(t, ".*", pRule.Pattern)
}

func TestBuildTagsPayload(t *testing.T) {
	assert.Equal(t, "-", string(BuildTagsPayload("", "", "")))
	assert.Equal(t, "[dd ddtags=\"hello:world\"]", string(BuildTagsPayload("hello:world", "", "")))
	assert.Equal(t, "[dd ddsource=\"nginx\"][dd ddsourcecategory=\"http_access\"][dd ddtags=\"hello:world, hi\"]", string(BuildTagsPayload("hello:world, hi", "nginx", "http_access")))
}

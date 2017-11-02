// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package processor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/stretchr/testify/assert"
)

func NewTestProcessor() Processor {
	return Processor{nil, nil, "", "", nil}
}

func buildTestProcessingRule(ruleType, replacePlaceholder, pattern string, p *Processor) config.IntegrationConfigLogSource {
	rule := config.LogsProcessingRule{
		Type:                    ruleType,
		Name:                    "test",
		ReplacePlaceholder:      replacePlaceholder,
		ReplacePlaceholderBytes: []byte(replacePlaceholder),
		Pattern:                 pattern,
		Reg:                     regexp.MustCompile(pattern),
	}
	return config.IntegrationConfigLogSource{ProcessingRules: []config.LogsProcessingRule{rule}, TagsPayload: []byte{'-'}}
}

func TestProcessor(t *testing.T) {
	var p *Processor
	p = New(nil, nil, "hello", "world")
	assert.Equal(t, "hello/world", string(p.apikeyString))
	p = New(nil, nil, "helloworld", "")
	assert.Equal(t, "helloworld", string(p.apikeyString))
}

func TestExclusion(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestProcessingRule("exclude_at_match", "", "world", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("world"), &source))
	assert.Equal(t, false, shouldProcess)

	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, false, shouldProcess)

	source = buildTestProcessingRule("exclude_at_match", "", "$world", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, true, shouldProcess)
}

func TestRedacting(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestProcessingRule("mask_sequences", "[masked_world]", "world", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("hello world!"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello [masked_world]!"), redactedMessage)

	source = buildTestProcessingRule("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("new test launched by User=beats@datadoghq.com on localhost"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("new test launched by [masked_user] on localhost"), redactedMessage)

	source = buildTestProcessingRule("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(message.NewNetworkMessage([]byte("The credit card 4323124312341234 was used to buy some time"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("The credit card [masked_credit_card] was used to buy some time"), redactedMessage)
}

func TestComputeExtraContent(t *testing.T) {
	p := NewTestProcessor()
	var extraContent []byte
	var extraContentParts []string

	source := &config.IntegrationConfigLogSource{TagsPayload: []byte{'-'}}
	extraContent = p.computeExtraContent(message.NewNetworkMessage([]byte("message"), source))
	extraContentParts = strings.Split(string(extraContent), " ")
	assert.Equal(t, 8, len(extraContentParts))
	format := "2006-01-02T15:04:05"
	assert.Equal(t, time.Now().UTC().Format(format), extraContentParts[1][:len(format)])

	extraContent = p.computeExtraContent(message.NewNetworkMessage([]byte("<message"), source))
	assert.Nil(t, extraContent)
}

func TestComputeApiKeyString(t *testing.T) {
	p := New(nil, nil, "hello", "world")

	source := &config.IntegrationConfigLogSource{}
	extraContent := p.computeApiKeyString(message.NewNetworkMessage(nil, source))
	assert.Equal(t, "hello/world", string(extraContent))

	source = &config.IntegrationConfigLogSource{Logset: "hi"}
	extraContent = p.computeApiKeyString(message.NewNetworkMessage(nil, source))
	assert.Equal(t, "hello/hi", string(extraContent))
}

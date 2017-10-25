// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package processor

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

// A Processor updates messages from an inputChan and pushes
// in an outputChan
type Processor struct {
	inputChan    chan message.Message
	outputChan   chan message.Message
	apikey       string
	logset       string
	apikeyString []byte
}

// New returns an initialized Processor
func New(inputChan, outputChan chan message.Message, apikey, logset string) *Processor {
	var apikeyString string
	if logset != "" {
		apikeyString = fmt.Sprintf("%s/%s", apikey, logset)
	} else {
		apikeyString = fmt.Sprintf("%s", apikey)
	}
	return &Processor{
		inputChan:    inputChan,
		outputChan:   outputChan,
		apikey:       apikey,
		logset:       logset,
		apikeyString: []byte(apikeyString),
	}
}

// Start starts the Processor
func (p *Processor) Start() {
	go p.run()
}

// run starts the processing of the inputChan
func (p *Processor) run() {
	for msg := range p.inputChan {
		shouldProcess, redactedMessage := p.applyRedactingRules(msg)
		if shouldProcess {
			extraContent := p.computeExtraContent(msg)
			apikeyString := p.computeApiKeyString(msg)
			payload := p.buildPayload(apikeyString, redactedMessage, extraContent)
			msg.SetContent(payload)
			p.outputChan <- msg
		}
	}
}

// computeExtraContent returns additional content to add to a log line.
// For instance, we want to add the timestamp, hostname and a log level
// to messages coming from a file
func (p *Processor) computeExtraContent(msg message.Message) []byte {
	if len(msg.Content()) > 0 && msg.Content()[0] != '<' {
		// fit RFC5424
		// <%pri%>%protocol-version% %timestamp:::date-rfc3339% %HOSTNAME% %$!new-appname% - - - %msg%\n
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000000+00:00")
		extraContent := []byte("<46>0 ")
		extraContent = append(extraContent, []byte(timestamp)...)
		extraContent = append(extraContent, ' ')
		extraContent = append(extraContent, []byte(config.LogsAgent.GetString("hostname"))...)
		extraContent = append(extraContent, ' ')
		service := msg.GetOrigin().LogSource.Service
		if service != "" {
			extraContent = append(extraContent, []byte(service)...)
		} else {
			extraContent = append(extraContent, '-')
		}
		extraContent = append(extraContent, []byte(" - - ")...)
		extraContent = append(extraContent, msg.GetOrigin().LogSource.TagsPayload...)
		extraContent = append(extraContent, ' ')
		return extraContent
	}
	return nil
}

func (p *Processor) computeApiKeyString(msg message.Message) []byte {
	sourceLogset := msg.GetOrigin().LogSource.Logset
	if sourceLogset != "" {
		return []byte(fmt.Sprintf("%s/%s", p.apikey, sourceLogset))
	}
	return p.apikeyString
}

// buildPayload returns a processed payload from a raw message
func (p *Processor) buildPayload(apikeyString, redactedMessage, extraContent []byte) []byte {
	payload := append(apikeyString, ' ')
	if extraContent != nil {
		payload = append(payload, extraContent...)
	}
	payload = append(payload, redactedMessage...)
	payload = append(payload, '\n')
	return payload
}

// applyRedactingRules returns given a message if we should process it or not,
// and a copy of the message with some fields redacted, depending on config
func (p *Processor) applyRedactingRules(msg message.Message) (bool, []byte) {
	content := msg.Content()
	for _, rule := range msg.GetOrigin().LogSource.ProcessingRules {
		switch rule.Type {
		case config.EXCLUDE_AT_MATCH:
			if rule.Reg.Match(content) {
				return false, nil
			}
		case config.MASK_SEQUENCES:
			content = rule.Reg.ReplaceAllLiteral(content, rule.ReplacePlaceholderBytes)
		}
	}
	return true, content
}

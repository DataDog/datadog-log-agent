// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/input/container"
	"github.com/DataDog/datadog-log-agent/pkg/input/listener"
	"github.com/DataDog/datadog-log-agent/pkg/input/tailer"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/DataDog/datadog-log-agent/pkg/processor"
	"github.com/DataDog/datadog-log-agent/pkg/sender"
)

const numberOfPipelines = 4
const chanSizes = 100

// Start starts the forwarder
func Start() {

	pipelinesEntryChannels := [](chan message.Message){}

	cm := sender.NewConnectionManager(
		config.LogsAgent.GetString("log_dd_url"),
		config.LogsAgent.GetInt("log_dd_port"),
		config.LogsAgent.GetBool("skip_ssl_validation"),
	)

	auditorChan := make(chan message.Message, chanSizes)
	a := auditor.New(auditorChan)
	a.Start()

	for i := 0; i < numberOfPipelines; i++ {

		senderChan := make(chan message.Message, chanSizes)
		f := sender.New(senderChan, auditorChan, cm)
		f.Start()

		processorChan := make(chan message.Message, chanSizes)
		p := processor.New(
			processorChan,
			senderChan,
			config.LogsAgent.GetString("api_key"),
			config.LogsAgent.GetString("logset"),
		)
		p.Start()

		pipelinesEntryChannels = append(pipelinesEntryChannels, processorChan)
	}

	// We want to share the load evenly on the pipelines for network and file sources.
	// A simple way to do so is to use the same channels but in a different order
	// This guarantees it will be almost evenly balanced, while keeping the logic simple.
	// If we need a third source at some point, we will need a better strategy
	filePipelinesEntryChannels := [](chan message.Message){}
	for i := numberOfPipelines - 1; i >= 0; i-- {
		filePipelinesEntryChannels = append(filePipelinesEntryChannels, pipelinesEntryChannels[i])
	}
	networkPipelinesEntryChannels := pipelinesEntryChannels

	l := listener.New(config.GetLogsSources(), networkPipelinesEntryChannels)
	l.Start()

	s := tailer.New(config.GetLogsSources(), filePipelinesEntryChannels, a)
	s.Start()

	// FIXME: As we have now 3 sources of logs, the comment on pipelines is not
	// accurate anymore. We need a better design for which pipeline to use
	// for any source
	c := container.New(config.GetLogsSources(), filePipelinesEntryChannels, a)
	c.Start()
}

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
	"github.com/DataDog/datadog-log-agent/pkg/pipeline"
	"github.com/DataDog/datadog-log-agent/pkg/sender"
)

// Start starts the forwarder
func Start() {

	cm := sender.NewConnectionManager(
		config.LogsAgent.GetString("log_dd_url"),
		config.LogsAgent.GetInt("log_dd_port"),
		config.LogsAgent.GetBool("skip_ssl_validation"),
	)

	auditorChan := make(chan message.Message, config.ChanSizes)
	a := auditor.New(auditorChan)
	a.Start()

	pp := pipeline.NewPipelineProvider()
	pp.Start(cm, auditorChan)

	l := listener.New(config.GetLogsSources(), pp)
	l.Start()

	s := tailer.New(config.GetLogsSources(), pp, a)
	s.Start()

	c := container.New(config.GetLogsSources(), pp, a)
	c.Start()
}

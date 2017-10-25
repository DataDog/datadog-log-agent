// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"log"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

// A Listener summons different protocol specific listeners based on configuration
type Listener struct {
	processorChans [](chan message.Message)
	sources        []*config.IntegrationConfigLogSource
}

// New returns an initialized Listener
func New(sources []*config.IntegrationConfigLogSource, processorChans [](chan message.Message)) *Listener {
	return &Listener{
		processorChans: processorChans,
		sources:        sources,
	}
}

// Start starts the Listener
func (l *Listener) Start() {
	sourceId := 0
	for _, source := range l.sources {
		switch source.Type {
		case config.TCP_TYPE:
			tcpl, err := NewTcpListener(l.processorChans[sourceId], source)
			sourceId = (sourceId + 1) % len(l.processorChans)
			if err != nil {
				log.Println("Can't start tcp source:", err)
			} else {
				tcpl.Start()
			}
		case config.UDP_TYPE:
			udpl, err := NewUdpListener(l.processorChans[sourceId], source)
			sourceId = (sourceId + 1) % len(l.processorChans)
			if err != nil {
				log.Println("Can't start udp source:", err)
			} else {
				udpl.Start()
			}
		default:
		}
	}
}

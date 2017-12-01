// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/pipeline"
)

// A Listener summons different protocol specific listeners based on configuration
type Listener struct {
	pp      *pipeline.PipelineProvider
	sources []*config.IntegrationConfigLogSource
}

// New returns an initialized Listener
func New(sources []*config.IntegrationConfigLogSource, pp *pipeline.PipelineProvider) *Listener {
	return &Listener{
		pp:      pp,
		sources: sources,
	}
}

// Start starts the Listener
func (l *Listener) Start() {
	for _, source := range l.sources {
		switch source.Type {
		case config.TCP_TYPE:
			tcpl, err := NewTcpListener(l.pp, source)
			if err != nil {
				log.Error("Can't start tcp source:", err)
			} else {
				tcpl.Start()
			}
		case config.UDP_TYPE:
			udpl, err := NewUdpListener(l.pp, source)
			if err != nil {
				log.Error("Can't start udp source:", err)
			} else {
				udpl.Start()
			}
		default:
		}
	}
}

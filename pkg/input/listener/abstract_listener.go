// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listener

import (
	"io"
	"net"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/DataDog/datadog-log-agent/pkg/pipeline"
)

// A NetworkListener implements the methods run and readMessages,
// required by the AbstractNetworkListener to run properly
type NetworkListener interface {
	run()
	readMessage(net.Conn, []byte) (int, error)
}

// AbstractNetworkListener is an abstracted network listener.
// It listens for bytes on a connection and forwards them to an output chan
type AbstractNetworkListener struct {
	listener NetworkListener
	pp       *pipeline.PipelineProvider
	source   *config.IntegrationConfigLogSource
}

// Start starts the AbstractNetworkListener
func (anl *AbstractNetworkListener) Start() {
	go anl.listener.run()
}

// forwardMessages lets the AbstractNetworkListener forward log messages to the output channel
func (anl *AbstractNetworkListener) forwardMessages(d *decoder.Decoder, outputChan chan message.Message) {
	for output := range d.OutputChan {
		if output.ShouldStop {
			return
		}

		netMsg := message.NewNetworkMessage(output.Content)
		o := message.NewOrigin()
		o.LogSource = anl.source
		netMsg.SetOrigin(o)
		outputChan <- netMsg
	}
}

// handleConnection listens to messages sent on a given connection
// and forwards them to an outputChan
func (anl *AbstractNetworkListener) handleConnection(conn net.Conn) {
	d := decoder.InitializeDecoder(anl.source)
	d.Start()
	go anl.forwardMessages(d, anl.pp.NextPipelineChan())
	for {
		inBuf := make([]byte, 4096)
		n, err := anl.listener.readMessage(conn, inBuf)
		if err == io.EOF {
			d.Stop()
			return
		}
		if err != nil {
			log.Error("Couldn't read message from connection:", err)
			d.Stop()
			return
		}
		d.InputChan <- decoder.NewInput(inBuf[:n])
	}
}

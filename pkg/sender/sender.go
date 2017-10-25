// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package sender

import (
	"net"

	"github.com/DataDog/datadog-log-agent/pkg/message"
)

const maxSubmissionAttempts = 5

// A Sender sends messages from an inputChan to datadog's intake,
// handling connections and retries
type Sender struct {
	inputChan   chan message.Message
	outputChan  chan message.Message
	connManager *ConnectionManager
	conn        net.Conn
}

// New returns an initialized Sender
func New(inputChan, outputChan chan message.Message, connManager *ConnectionManager) *Sender {
	return &Sender{
		inputChan:   inputChan,
		outputChan:  outputChan,
		connManager: connManager,
	}
}

// Start starts the Sender
func (s *Sender) Start() {
	go s.run()
}

// run lets the sender wire messages
func (s *Sender) run() {
	for payload := range s.inputChan {
		s.wireMessage(payload)
	}
}

// wireMessage lets the Sender send a message to datadog's intake
func (s *Sender) wireMessage(payload message.Message) {
	retries := maxSubmissionAttempts

	for retries > 0 {

		if s.conn == nil {
			conn, err := s.connManager.NewConnection() // blocks until a new conn is ready
			if err != nil {
				reportNetError("out_connection.connection_failure", err)
				return
			}
			s.conn = conn
		}
		_, err := s.conn.Write(payload.Content())
		if err != nil {
			if err.Error() != "tls: use of closed connection" {
				// no need to report this error, it's expected
				// for connection to be closed when unused
				reportNetError("message.submit_error", err)
			}
			retries -= 1
			s.connManager.CloseConnection(s.conn)
			s.conn = nil
			continue
		}

		s.outputChan <- payload
		return
	}
}

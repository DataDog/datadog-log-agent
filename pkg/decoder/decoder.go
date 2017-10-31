// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

// Payload represents a list of bytes and an optional reference to its origin
type Payload struct {
	content []byte
	offset  int64
}

// NewPayload returns a new decoder payload
func NewPayload(content []byte, offset int64) *Payload {
	return &Payload{content, offset}
}

// Decoder splits raw data based on `\n`, and sends those messages to a channel
type Decoder struct {
	InputChan  chan *Payload
	OutputChan chan message.Message
	msgBuffer  *bytes.Buffer
}

// InitializeDecoder returns a properly initialized Decoder
func InitializedDecoder() *Decoder {
	inputChan := make(chan *Payload)
	outputChan := make(chan message.Message)
	return New(inputChan, outputChan)
}

// New returns an initialized Decoder
func New(InputChan chan *Payload, OutputChan chan message.Message) *Decoder {
	var msgBuf bytes.Buffer
	return &Decoder{
		InputChan:  InputChan,
		OutputChan: OutputChan,
		msgBuffer:  &msgBuf,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	go d.run()
}

// run lets the Decoder handle data coming from the InputChan
func (d *Decoder) run() {
	for data := range d.InputChan {
		d.decodeIncomingData(data.content, data.offset)
	}
	d.OutputChan <- message.NewStopMessage()
}

// Stop stops the Decoder
func (d *Decoder) Stop() {
	close(d.InputChan)
}

var truncatedMsg = []byte("...TRUNCATED...")
var truncatedLen = len(truncatedMsg)
var maxMessageLen = config.MaxMessageLen - truncatedLen

// sendBuffuredMessage flushes the buffer and sends the message
func (d *Decoder) sendBuffuredMessage(offset int64) {
	msg := make([]byte, d.msgBuffer.Len())
	// d.msgBuffer.Bytes() returns a slice to the []byte, we thus need to copy it
	copy(msg, d.msgBuffer.Bytes())
	if len(msg) > 0 {
		m := message.NewMessage(msg)
		o := message.NewOrigin()
		o.Offset = offset
		m.SetOrigin(o)
		d.OutputChan <- m
	}
	d.msgBuffer.Reset()
}

// decodeIncomingData splits raw data based on `\n`, creates and sends messages to a channel
func (d *Decoder) decodeIncomingData(inBuf []byte, offset int64) {
	var i, j = 0, 0
	var maxj = maxMessageLen - d.msgBuffer.Len()
	// Note: we will truncate messages of length MaxLen - truncatedLen
	// instead of MaxLen. We'll live with it for now
	for ; j < len(inBuf); j++ {
		if inBuf[j] == '\n' {
			d.msgBuffer.Write(inBuf[i:j])
			d.sendBuffuredMessage(offset + int64(j+1))
			i = j + 1 // +1 as we skip the `\n`
			maxj = maxMessageLen - d.msgBuffer.Len()
		} else if j == maxj {
			d.msgBuffer.Write(inBuf[i:j])
			d.msgBuffer.Write(truncatedMsg)
			d.sendBuffuredMessage(offset + int64(j))
			d.msgBuffer.Write(truncatedMsg)
			i = j
		}
	}
	d.msgBuffer.Write(inBuf[i:j])
}

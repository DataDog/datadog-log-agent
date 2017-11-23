// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"

	"github.com/DataDog/datadog-log-agent/pkg/config"
)

// maxMessageLen represents the maximum length for a message
var maxMessageLen = config.MaxMessageLen - len([]byte(config.TruncWarningMsg))

// Payload represents a list of bytes consumed by the Decoder
type Payload struct {
	content []byte
}

// NewPayload returns a new decoder payload
func NewPayload(content []byte) *Payload {
	return &Payload{content}
}

// Message represents a list of bytes produced by the Decoder
type Message struct {
	Content            []byte
	IsTruncated        bool
	NumberOfCharacters int
	IsStop             bool
}

// // NewPayload returns a new decoder message
func newMessage(content []byte, isTruncated bool, numberOfCharacters int) *Message {
	return &Message{
		Content:            content,
		IsTruncated:        isTruncated,
		NumberOfCharacters: numberOfCharacters,
	}
}

// newMessageStop returns a new decoder message stop
func newMessageStop() *Message {
	return &Message{IsStop: true}
}

// Decoder splits raw data into lines and passes them along to a messageProducer to emit new messages
type Decoder struct {
	InputChan  chan *Payload
	OutputChan chan *Message

	lineBuffer  *bytes.Buffer
	msgProducer messageProducer
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.IntegrationConfigLogSource) *Decoder {
	inputChan := make(chan *Payload)
	outputChan := make(chan *Message)

	var msgProducer messageProducer
	for _, rule := range source.ProcessingRules {
		switch rule.Type {
		case config.MULTILINE:
			msgProducer = newMultiLineMessageProducer(outputChan, rule.Reg)
		}
	}
	if msgProducer == nil {
		msgProducer = newSingleLineMessageProducer(outputChan)
	}

	return New(inputChan, outputChan, msgProducer)
}

// New returns an initialized Decoder
func New(InputChan chan *Payload, OutputChan chan *Message, msgProducer messageProducer) *Decoder {
	var lineBuffer bytes.Buffer
	return &Decoder{
		InputChan:   InputChan,
		OutputChan:  OutputChan,
		lineBuffer:  &lineBuffer,
		msgProducer: msgProducer,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	go d.run()
}

// Stop stops the Decoder
func (d *Decoder) Stop() {
	close(d.InputChan)
}

// run lets the Decoder handle data coming from the InputChan
func (d *Decoder) run() {
	d.msgProducer.Start()
	for data := range d.InputChan {
		d.decodeIncomingData(data.content)
	}
	d.msgProducer.Stop()
	d.OutputChan <- newMessageStop()
}

// decodeIncomingData splits raw data based on `\n`, creates and processes new lines
func (d *Decoder) decodeIncomingData(inBuf []byte) {
	i, j := 0, 0
	n := len(inBuf)
	maxj := i + maxMessageLen - d.lineBuffer.Len()

	for ; j < n; j++ {
		if j == maxj {
			// process the line as it is too long
			d.lineBuffer.Write(inBuf[i:j])
			d.processLine()
			i = j
			maxj = i + maxMessageLen
		} else if inBuf[j] == '\n' {
			d.lineBuffer.Write(inBuf[i:j])
			d.processLine()
			i = j + 1 // +1 as we skip the `\n`
			maxj = i + maxMessageLen
		}
	}
	d.lineBuffer.Write(inBuf[i:j])
}

// processLine delegates its work to msgProducer
func (d *Decoder) processLine() {
	newLine := make([]byte, d.lineBuffer.Len())
	copy(newLine, d.lineBuffer.Bytes())
	d.msgProducer.Consume(newLine)
	d.lineBuffer.Reset()
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

// countDownDuration represents the time after which the buffered line is sent when there is no more incoming data
const countDownDuration = 1 * time.Second

// maxMessageLen represents the maximum length for a message
var maxMessageLen = config.MaxMessageLen

// Payload represents a list of bytes
type Payload struct {
	content []byte
}

// NewPayload returns a new decoder payload
func NewPayload(content []byte) *Payload {
	return &Payload{content}
}

// Decoder splits raw data into messages using '\n' for single-line logs and lineRe for multi-line logs
// and sends them to a channel
type Decoder struct {
	InputChan  chan *Payload
	OutputChan chan message.Message

	msgBuffer  *bytes.Buffer
	lineBuffer *bytes.Buffer

	timer  *time.Timer
	lineRe *regexp.Regexp
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.IntegrationConfigLogSource) *Decoder {
	var lineRe *regexp.Regexp
	for _, rule := range source.ProcessingRules {
		switch rule.Type {
		case config.MULTILINE:
			lineRe = rule.Reg
		}
	}

	inputChan := make(chan *Payload)
	outputChan := make(chan message.Message)
	return New(inputChan, outputChan, lineRe)
}

// New returns an initialized Decoder
func New(InputChan chan *Payload, OutputChan chan message.Message, lineRe *regexp.Regexp) *Decoder {
	var msgBuffer, lineBuffer bytes.Buffer
	return &Decoder{
		InputChan:  InputChan,
		OutputChan: OutputChan,
		msgBuffer:  &msgBuffer,
		lineBuffer: &lineBuffer,
		lineRe:     lineRe,
	}
}

// isMultiLineEnabled returns true if the decoder supports multi-line logs
func (d *Decoder) isMultiLineEnabled() bool {
	return d.lineRe != nil
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
	for data := range d.InputChan {
		if d.isMultiLineEnabled() {
			d.ackIncomingData()
			endedWithEOL := d.decodeIncomingData(data.content)
			if endedWithEOL {
				d.ackEndIncomingData()
			}
		} else {
			d.decodeIncomingData(data.content)
		}

	}
	d.OutputChan <- message.NewStopMessage()
}

// ackIncomingData stops the timer which flushes the multi-line buffer as we got new data
func (d *Decoder) ackIncomingData() {
	if d.timer != nil {
		d.timer.Stop()
	}
}

// ackEndIncomingData starts the timer which flushes the multi-line buffer
func (d *Decoder) ackEndIncomingData() {
	d.timer = time.AfterFunc(countDownDuration, func() {
		d.sendMessage()
	})
}

// decodeIncomingData splits raw data based on `\n`, creates and processes new lines
// returns true if inBuf ends with `\n`
func (d *Decoder) decodeIncomingData(inBuf []byte) (endsWithEOL bool) {
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

	// check if inBuf ends with `\n`
	if len(inBuf) > 0 && inBuf[n-1] == '\n' {
		endsWithEOL = true
	}
	return endsWithEOL
}

// processLine appends new line to the message, sends and truncates messages
func (d *Decoder) processLine() {
	line := d.lineBuffer.Bytes()
	defer d.lineBuffer.Reset()

	isLineAdded := false
	if d.isMultiLineEnabled() {
		if d.lineRe.Match(line) {
			d.sendMessage()
		}
		isLineAdded = d.appendLine(line)
	} else {
		isLineAdded = d.appendLine(line)
		if isLineAdded {
			d.sendMessage()
		}
	}
	if !isLineAdded {
		d.truncateAndSendMessage(line)
	}
}

// appendLine attemps to add the new line to the message if there is enough space in msgBuf
// returns true if the line is added to the message
func (d *Decoder) appendLine(line []byte) bool {
	maxLineLen := maxMessageLen - d.msgBuffer.Len()
	if len(line) < maxLineLen {
		if d.msgBuffer.Len() != 0 {
			d.msgBuffer.Write([]byte(`\n`))
		}
		d.msgBuffer.Write(line)
		return true
	}
	return false
}

// truncateAndSendMessage appends the new line to to msgBuf and sends the message
// the order of the operations changes for multi-line logs
func (d *Decoder) truncateAndSendMessage(line []byte) {
	if d.isMultiLineEnabled() {
		d.sendMessage()
		d.msgBuffer.Write(line)
	} else {
		d.msgBuffer.Write(line)
		d.sendMessage()
	}
}

// sendMessage sends the message and flushes msgBuf
func (d *Decoder) sendMessage() {
	msg := make([]byte, d.msgBuffer.Len())
	copy(msg, d.msgBuffer.Bytes())
	defer d.msgBuffer.Reset()

	if len(msg) > 0 {
		m := message.NewMessage(msg)
		d.OutputChan <- m
	}
}

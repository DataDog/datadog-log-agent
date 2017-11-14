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

// defaultCountDownDuration represents the time after which the buffered line is sent when there is no more incoming data
const defaultCountDownDuration = 1 * time.Second

// Payload represents a list of bytes and an optional reference to its origin

type Payload struct {
	content []byte
	offset  int64 // when set, offset represents the position of the data in a file
}

// NewPayload returns a new decoder payload
func NewPayload(content []byte, offset int64) *Payload {
	return &Payload{content, offset}
}

// Decoder splits raw data into messages using '\n' for single-line logs and re for multi-line logs
// and sends them to a channel
type Decoder struct {
	InputChan         chan *Payload
	OutputChan        chan message.Message
	countDownDuration time.Duration
	isMultiLine       bool
	lineBuffer        *bytes.Buffer
	msgBuffer         *bytes.Buffer
	newLineRe         *regexp.Regexp
	timer             *time.Timer
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.IntegrationConfigLogSource) *Decoder {
	var isMultiLine bool = false
	var newLineRe *regexp.Regexp
	for _, rule := range source.ProcessingRules {
		switch rule.Type {
		case config.MULTILINE:
			isMultiLine = true
			newLineRe = rule.Reg
		}
	}

	inputChan := make(chan *Payload)
	outputChan := make(chan message.Message)
	return New(inputChan, outputChan, defaultCountDownDuration, isMultiLine, newLineRe)
}

// New returns an initialized Decoder
func New(InputChan chan *Payload, OutputChan chan message.Message, countDownDuration time.Duration, isMultiLine bool, newLineRe *regexp.Regexp) *Decoder {
	var lineBuffer, msgBuf bytes.Buffer
	return &Decoder{
		InputChan:         InputChan,
		OutputChan:        OutputChan,
		countDownDuration: countDownDuration,
		isMultiLine:       isMultiLine,
		lineBuffer:        &lineBuffer,
		msgBuffer:         &msgBuf,
		newLineRe:         newLineRe,
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
	for data := range d.InputChan {
		d.stopCountDown()
		if endsWithEOL, offset := d.decodeIncomingData(data.content, data.offset); endsWithEOL {
			d.restartCountDown(offset)
		}
	}
	d.OutputChan <- message.NewStopMessage()
}

// stopCountDown prevents the timer for firing
func (d *Decoder) stopCountDown() {
	if d.isMultiLine && d.timer != nil {
		d.timer.Stop()
	}
}

// restartCountDown starts the timer and sends the last line when it fires
func (d *Decoder) restartCountDown(offset int64) {
	if d.isMultiLine {
		d.timer = time.AfterFunc(d.countDownDuration, func() {
			newLine := make([]byte, d.lineBuffer.Len())
			copy(newLine, d.lineBuffer.Bytes())
			defer d.lineBuffer.Reset()
			d.msgBuffer.Write(newLine)
			d.sendBufferedMessage(offset)
		})
	}
}

var truncatedMsg = []byte("...TRUNCATED...")
var truncatedLen = len(truncatedMsg)
var maxMessageLen = config.MaxMessageLen - truncatedLen

// sendBufferedMessage flushes the buffer and sends the message
func (d *Decoder) sendBufferedMessage(offset int64) {
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

// processBufferedLine checks the new line, appends the whole line or just a piece to the message
// and sends the message if needed
func (d *Decoder) processNewLine(offset int64) {
	newLine := make([]byte, d.lineBuffer.Len())
	copy(newLine, d.lineBuffer.Bytes())
	defer d.lineBuffer.Reset()

	if !d.isMultiLine {
		d.msgBuffer.Write(newLine)
		d.sendBufferedMessage(offset)
		return
	}

	if d.msgBuffer.Len() == 0 {
		d.msgBuffer.Write(newLine)
		return
	}

	if d.newLineRe.Match(newLine) {
		d.sendBufferedMessage(offset - int64(1+len(newLine))) // remove '\n' + new line length
		d.msgBuffer.Write(newLine)
		return
	}

	maxLen := maxMessageLen - d.msgBuffer.Len() - 2
	if len(newLine) >= maxLen {
		d.msgBuffer.Write([]byte(`\n`))
		d.msgBuffer.Write(newLine[:maxLen])
		d.msgBuffer.Write(truncatedMsg)
		d.sendBufferedMessage(offset - int64(1+len(newLine[maxLen:]))) // remove '\n' + the length of the piece of the line that can't fit
		d.msgBuffer.Write(truncatedMsg)
		d.msgBuffer.Write(newLine[maxLen:])
		return
	}

	// append a new line to the message
	d.msgBuffer.Write([]byte(`\n`))
	d.msgBuffer.Write(newLine)
}

// recoverTooLongBufferedLine truncates the new line and sends its left part and the message if needed
func (d *Decoder) recoverTooLongBufferedLine(offset int64) {
	newLine := make([]byte, d.lineBuffer.Len())
	copy(newLine, d.lineBuffer.Bytes())
	defer d.lineBuffer.Reset()

	// truncate and send new line
	truncAndSendLine := func() {
		newLine = append(newLine, truncatedMsg...)
		d.msgBuffer.Write(newLine)
		d.sendBufferedMessage(offset)
		d.msgBuffer.Write(truncatedMsg)
	}

	if !d.isMultiLine || d.msgBuffer.Len() == 0 {
		truncAndSendLine()
		return
	}

	if d.newLineRe.Match(newLine) {
		d.sendBufferedMessage(offset - int64(len(newLine)))
		truncAndSendLine()
		return
	}

	maxLen := maxMessageLen - d.msgBuffer.Len() - 1
	d.msgBuffer.Write([]byte(`\n`))
	d.msgBuffer.Write(newLine[:maxLen])
	d.msgBuffer.Write(truncatedMsg)
	d.sendBufferedMessage(offset - int64(len(newLine[maxLen:]))) // remove the length of the line that can't fit
	d.msgBuffer.Write(truncatedMsg)
	d.msgBuffer.Write(newLine[maxLen:])
}

// decodeIncomingData splits raw data based on `\n`, creates and processes new lines
// returns true and the new offset if the end of inBuf is likely to be the end of a message
func (d *Decoder) decodeIncomingData(inBuf []byte, offset int64) (endsWithEOL bool, newOffset int64) {
	var i, j = 0, 0
	var maxj = maxMessageLen - d.lineBuffer.Len()
	// Note: we will truncate messages of length MaxLen - truncatedLen
	// instead of MaxLen. We'll live with it for now
	for ; j < len(inBuf); j++ {
		if inBuf[j] == '\n' {
			d.lineBuffer.Write(inBuf[i:j])
			d.processNewLine(offset + int64(j+1))
			i = j + 1 // +1 as we skip the `\n`
			maxj = maxMessageLen - d.lineBuffer.Len()
		} else if j == maxj {
			d.lineBuffer.Write(inBuf[i:j])
			d.recoverTooLongBufferedLine(offset + int64(j))
			i = j
			maxj = maxMessageLen - d.lineBuffer.Len()
		}
	}
	d.lineBuffer.Write(inBuf[i:j])

	if len(inBuf) > 0 && inBuf[j-1] == '\n' {
		endsWithEOL = true
	}
	newOffset = offset + int64(j)
	return endsWithEOL, newOffset
}

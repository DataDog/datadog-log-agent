// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"errors"
	"regexp"
	"time"
)

// countDownDuration represents the time after which the buffered message is sent
const countDownDuration = 1 * time.Second

// messageProducer consumes new lines to compute and emit messages
// Prepare and Dispose can be used for special needs
type messageProducer interface {
	Consume(line []byte)
	Prepare()
	Dispose()
}

// singleLineMessageProducer emits new messages whenever it receives new lines
type singleLineMessageProducer struct {
	outputChan chan *Message
	isMsgTrunc bool
}

// newSingleLineMessageProducer returns a new singleLineMessageProducer
func newSingleLineMessageProducer(outputChan chan *Message) *singleLineMessageProducer {
	return &singleLineMessageProducer{
		outputChan: outputChan,
	}
}

// Consume checks line and the inner state of the producer to emit new messages
func (p singleLineMessageProducer) Consume(line []byte) {
	if len(line) == 0 {
		return
	}
	var msg *Message
	if len(line) < maxMessageLen {
		msg = newMessage(line, p.isMsgTrunc)
		p.isMsgTrunc = false
	} else {
		msg = newMessage(line, true)
		p.isMsgTrunc = true
	}
	p.outputChan <- msg
}

// Prepare is not needed for single line messages
func (p singleLineMessageProducer) Prepare() {
	// not implemented
}

// Dispose is not needed for single line messages
func (p singleLineMessageProducer) Dispose() {
	// not implemented
}

// multiLineMessageProducer emits new multi-line messages
type multiLineMessageProducer struct {
	outputChan     chan *Message
	contentBuffer  *bytes.Buffer
	newLineRe      *regexp.Regexp
	isContentTrunc bool
	timer          *time.Timer
}

// newMultiLineMessageProducer returns a new newMultiLineMessageProducer
func newMultiLineMessageProducer(outputChan chan *Message, newLineRe *regexp.Regexp) *multiLineMessageProducer {
	var contentBuffer bytes.Buffer
	return &multiLineMessageProducer{
		outputChan:    outputChan,
		contentBuffer: &contentBuffer,
		newLineRe:     newLineRe,
	}
}

// Prepare stops the timer that sends the content in buffer
func (p multiLineMessageProducer) Prepare() {
	if p.timer != nil {
		p.timer.Stop()
	}
}

// Dispose starts the timer that sends the content in buffer
func (p multiLineMessageProducer) Dispose() {
	p.timer = time.AfterFunc(countDownDuration, func() {
		p.sendMessage()
	})
}

// Consume appends new line to the content in buffer or sends or truncates the content
func (p multiLineMessageProducer) Consume(line []byte) {
	var appendError error
	if p.newLineRe.Match(line) {
		p.sendMessage()
		appendError = p.appendLine(line)
	} else {
		appendError = p.appendLine(line)
	}
	if appendError != nil {
		p.truncateMessage(line)
	}
}

// appendLine attemps to add the new line to the content in buffer if there is enough space
// returns an error if the line could not be added
func (p multiLineMessageProducer) appendLine(line []byte) error {
	maxLineLen := maxMessageLen - p.contentBuffer.Len()
	if len(line) < maxLineLen {
		if p.contentBuffer.Len() != 0 {
			p.contentBuffer.Write([]byte(`\n`))
		}
		p.contentBuffer.Write(line)
		return nil
	}
	return errors.New("could not append new line, not enough space left")
}

// truncateMessage sends the content in buffer and keeps the new line in buffer
func (p multiLineMessageProducer) truncateMessage(line []byte) {
	p.isContentTrunc = true
	p.sendMessage()
	p.isContentTrunc = true
	p.contentBuffer.Write(line)
}

// sendMessage sends the content in buffer and flushes the buffer
func (p multiLineMessageProducer) sendMessage() {
	content := make([]byte, p.contentBuffer.Len())
	copy(content, p.contentBuffer.Bytes())

	if len(content) > 0 {
		isTruncated := p.isContentTrunc
		msg := newMessage(content, isTruncated)
		p.outputChan <- msg
	}

	p.isContentTrunc = false
	p.contentBuffer.Reset()
}

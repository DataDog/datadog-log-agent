// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"errors"
	"regexp"
	"sync"
	"time"
)

// messageProducer consumes new lines to compute and emit messages
// Prepare and Dispose can be used for special needs
type messageProducer interface {
	Start()
	Stop()
	Consume(line []byte)
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

// Start is not needed for single line messages
func (p *singleLineMessageProducer) Start() {
	// not implemented
}

// Stop is not needed for single line messages
func (p *singleLineMessageProducer) Stop() {
	// not implemented
}

// Consume checks line and the inner state of the producer to emit new messages
func (p *singleLineMessageProducer) Consume(line []byte) {
	lineLen := len(line)
	if lineLen == 0 {
		return
	}
	var msg *Message
	if lineLen < maxMessageLen {
		msg = newMessage(line, p.isMsgTrunc, lineLen+1) // add 1 for `\n`
		p.isMsgTrunc = false
	} else {
		msg = newMessage(line, true, lineLen)
		p.isMsgTrunc = true
	}
	p.outputChan <- msg
}

// outdatedDuration represents the time after which the buffered message is sent
const outdatedDuration = 1 * time.Second

// multiLineMessageProducer emits new multi-line messages
type multiLineMessageProducer struct {
	outputChan    chan *Message
	contentBuffer *bytes.Buffer
	newLineRe     *regexp.Regexp
	timer         *time.Timer
	consumeMutex  sync.Mutex

	isContentTrunc  bool
	charactersCount int
}

// newMultiLineMessageProducer returns a new newMultiLineMessageProducer
func newMultiLineMessageProducer(outputChan chan *Message, newLineRe *regexp.Regexp) *multiLineMessageProducer {
	var contentBuffer bytes.Buffer
	timer := time.NewTimer(outdatedDuration)
	return &multiLineMessageProducer{
		outputChan:    outputChan,
		contentBuffer: &contentBuffer,
		newLineRe:     newLineRe,
		timer:         timer,
	}
}

// Start consumes timer events to send the content in buffer if it's outdated
func (p *multiLineMessageProducer) Start() {
	go func() {
		for range p.timer.C {
			p.consumeMutex.Lock()
			p.sendMessage()
			p.consumeMutex.Unlock()
		}
	}()
}

// Stop stops the timer
func (p *multiLineMessageProducer) Stop() {
	p.timer.Stop()
}

// Consume appends new line to the content in buffer or sends or truncates the content
func (p *multiLineMessageProducer) Consume(line []byte) {
	p.consumeMutex.Lock()
	p.timer.Stop()
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
	p.timer.Reset(outdatedDuration)
	p.consumeMutex.Unlock()
}

// appendLine attemps to add the new line to the content in buffer if there is enough space
// returns an error if the line could not be added
func (p *multiLineMessageProducer) appendLine(line []byte) error {
	lineLen := len(line)
	maxLineLen := maxMessageLen - p.contentBuffer.Len()
	if lineLen < maxLineLen {
		if p.contentBuffer.Len() != 0 {
			p.contentBuffer.Write([]byte(`\n`))
		}
		p.contentBuffer.Write(line)
		p.charactersCount += lineLen + 1 // add 1 for '\n'
		return nil
	}
	return errors.New("could not append new line, not enough space left")
}

// truncateMessage sends the content in buffer and keeps the new line in buffer
func (p *multiLineMessageProducer) truncateMessage(line []byte) {
	p.isContentTrunc = true
	p.sendMessage()
	p.isContentTrunc = true
	p.contentBuffer.Write(line)
	lineLen := len(line)
	if lineLen < maxMessageLen {
		p.charactersCount += lineLen + 1 // add 1 for '\n'
	} else {
		p.charactersCount += lineLen
	}
}

// sendMessage sends the content in buffer and flushes the buffer
func (p *multiLineMessageProducer) sendMessage() {
	content := make([]byte, p.contentBuffer.Len())
	copy(content, p.contentBuffer.Bytes())

	if len(content) > 0 {
		isTruncated := p.isContentTrunc
		msg := newMessage(content, isTruncated, p.charactersCount)
		p.outputChan <- msg
	}
	p.reset()
}

// reset clears the context to process new messages
func (p *multiLineMessageProducer) reset() {
	p.charactersCount = 0
	p.contentBuffer.Reset()
	p.isContentTrunc = false
}

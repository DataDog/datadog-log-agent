// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

// PayloadContext can create new message.MessageOrigin from a msg
// and be updated with content
type PayloadContext interface {
	messageOrigin() *message.MessageOrigin
	update(content []byte)
}

// Payload represents a list of bytes with a context
type Payload struct {
	content []byte
	context PayloadContext
}

// NewPayload returns a new decoder payload
func NewPayload(content []byte, context PayloadContext) *Payload {
	return &Payload{content, context}
}

// FileContext is an implementation of PayloadContext for files
// it holds the offset from where we should seek in the file when restarting logs-agent
type FileContext struct {
	offset int64
}

// NewFileContext creates a new FileContext
func NewFileContext(offset int64) *FileContext {
	return &FileContext{offset}
}

// messageOrigin computes a new message.MessageOrigin with an offset
// pointing to where we should seek in the file when restrarting logs-agent
func (c FileContext) messageOrigin() *message.MessageOrigin {
	o := message.NewOrigin()
	o.Offset = c.offset
	return o
}

// update changes the offset to point to the end of the content
func (c *FileContext) update(content []byte) {
	offset := c.offset + int64(len(content))
	c.offset = offset
}

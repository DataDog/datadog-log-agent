// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

type MockPayloadContext struct {
	Content []byte
	Message []byte
}

func (c *MockPayloadContext) messageOrigin() *message.MessageOrigin {
	c.Message = make([]byte, len(c.Content))
	copy(c.Message, c.Content)
	c.Content = c.Content[:0]
	return nil
}

func (c *MockPayloadContext) update(content []byte) {
	c.Content = append(c.Content, content...)
}

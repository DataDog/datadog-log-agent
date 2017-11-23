// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package message

import (
	"github.com/DataDog/datadog-log-agent/pkg/config"
)

// Message represents a log line sent to datadog, with its metadata
type Message interface {
	Content() []byte
	SetContent([]byte)
	IsTruncated() bool
	GetOrigin() *MessageOrigin
	SetOrigin(*MessageOrigin)
}

// MessageOrigin represents the Origin of a message
type MessageOrigin struct {
	Identifier string
	LogSource  *config.IntegrationConfigLogSource
	Offset     int64
	Timestamp  string
}

type message struct {
	content     []byte
	isTruncated bool
	Origin      *MessageOrigin
}

// Content returns the content the message, the actual log line
func (m *message) Content() []byte {
	return m.content
}

// SetContent updates the content the message
func (m *message) SetContent(content []byte) {
	m.content = content
}

// IsTruncated returns true if the log is truncated
func (m *message) IsTruncated() bool {
	return m.isTruncated
}

// GetOrigin returns the Origin from which the message comes
func (m *message) GetOrigin() *MessageOrigin {
	return m.Origin
}

// SetOrigin sets the integration from which the message comes
func (m *message) SetOrigin(Origin *MessageOrigin) {
	m.Origin = Origin
}

// NewMessage returns a new message
func NewMessage(content []byte, isTruncated bool) *message {
	return &message{
		content:     content,
		isTruncated: isTruncated,
	}
}

// NewFileOrigin returns a new MessageOrigin
func NewOrigin() *MessageOrigin {
	return &MessageOrigin{}
}

// StopMessage is used to let a component stop gracefully
type StopMessage struct {
	*message
}

func NewStopMessage() *StopMessage {
	return &StopMessage{
		message: NewMessage(nil, false),
	}
}

// FileMessage is a message coming from a File
type FileMessage struct {
	*message
}

func NewFileMessage(content []byte, isTruncated bool) *FileMessage {
	return &FileMessage{
		message: NewMessage(content, isTruncated),
	}
}

// FileMessage is a message coming from a network Source
type NetworkMessage struct {
	*message
}

func NewNetworkMessage(content []byte, isTruncated bool) *NetworkMessage {
	return &NetworkMessage{
		message: NewMessage(content, isTruncated),
	}
}

// ContainerMessage is a message coming from a container Source
type ContainerMessage struct {
	*message
}

func NewContainerMessage(content []byte, isTruncated bool) *ContainerMessage {
	return &ContainerMessage{
		message: NewMessage(content, isTruncated),
	}
}

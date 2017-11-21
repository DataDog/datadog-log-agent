// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/stretchr/testify/assert"
)

func TestDecodeIncomingDataForSingleLineLogs(t *testing.T) {
	outChan := make(chan message.Message, 10)
	d := New(nil, outChan, nil)

	var out message.Message

	// multiple messages in one buffer
	d.decodeIncomingData([]byte("helloworld\n"), nil)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	d.decodeIncomingData([]byte("helloworld\nhowayou\ngoodandyou"), nil)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "howayou", string(out.Content()))
	assert.Equal(t, "goodandyou", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())
	d.lineBuffer.Reset()

	// messages overflow in the next buffer
	d.decodeIncomingData([]byte("helloworld\nthisisa"), nil)
	assert.Equal(t, "thisisa", d.lineBuffer.String())
	d.decodeIncomingData([]byte("longinput\nindeed"), nil)
	out = <-outChan
	out = <-outChan
	assert.Equal(t, "thisisalonginput", string(out.Content()))
	assert.Equal(t, "indeed", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// edge cases, do not crash
	d.decodeIncomingData([]byte("\n\n"), nil)
	d.decodeIncomingData([]byte(""), nil)

	// buffer overflow
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte("hello world"), nil)
	d.decodeIncomingData([]byte("!\n"), nil)
	out = <-outChan
	assert.Equal(t, "hello world!", string(out.Content()))

	// message too big
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// message too big, over several calls
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen-5)), nil)
	d.decodeIncomingData([]byte(strings.Repeat("a", 25)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 20), string(out.Content()))

	// message twice too big
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte(strings.Repeat("a", 2*maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// message twice too big, over several calls
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen+5)), nil)
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 10), string(out.Content()))

	// decoder context management
	var mockPayloadContext MockPayloadContext

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld\nthisisa"), &mockPayloadContext)
	assert.Equal(t, "helloworld\n", string(mockPayloadContext.Message))
	assert.Equal(t, "", string(mockPayloadContext.Content))

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld"), &mockPayloadContext)
	assert.Equal(t, "", string(mockPayloadContext.Content))
	assert.Equal(t, "", string(mockPayloadContext.Message))

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen+5)), &mockPayloadContext)
	assert.Equal(t, maxMessageLen, len(mockPayloadContext.Message))
	assert.Equal(t, "", string(mockPayloadContext.Content))
}

func TestDecodeIncomingDataForMultiLineLogs(t *testing.T) {
	inChan := make(chan *Payload, 10)
	outChan := make(chan message.Message, 10)
	re := regexp.MustCompile("[0-9]+\\.")
	d := New(inChan, outChan, re)

	var out message.Message
	go d.run()

	// two lines message in one raw data
	inChan <- NewPayload([]byte("1. Hello\nworld!\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple messages in one raw data
	inChan <- NewPayload([]byte("1. Hello\nworld!\n2. How are you\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// two lines message over two raw data
	inChan <- NewPayload([]byte("1. Hello\n"), nil)
	inChan <- NewPayload([]byte("world!\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple messages accross two raw data
	inChan <- NewPayload([]byte("1. Hello\n"), nil)
	inChan <- NewPayload([]byte("world!\n2. How are you\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// single-line message in one raw data
	inChan <- NewPayload([]byte("1. Hello world!\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple single-line messages in one raw data
	inChan <- NewPayload([]byte("1. Hello world!\n2. How are you\n"), nil)
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// two lines big message in one raw data
	inChan <- NewPayload([]byte("12345678.\n"+strings.Repeat("a", maxMessageLen-5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, "12345678.", string(out.Content()))
	out = <-outChan
	assert.Equal(t, +maxMessageLen-5, len(out.Content()))

	// two lines big message over two raw data
	inChan <- NewPayload([]byte("12345678.\n"), nil)
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen-5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, "12345678.", string(out.Content()))
	out = <-outChan
	assert.Equal(t, +maxMessageLen-5, len(out.Content()))

	// two lines too big message in one raw data
	inChan <- NewPayload([]byte("12345678.\n"+strings.Repeat("a", maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, "12345678.", string(out.Content()))
	out = <-outChan
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// single-line big message over two raw data
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen)), nil)
	inChan <- NewPayload([]byte(strings.Repeat("a", 5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// single-line too big message in one raw data
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// message twice too big in one raw data
	inChan <- NewPayload([]byte(strings.Repeat("a", 2*maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 5), string(out.Content()))

	// message twice too big over two raw data
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen+5)), nil)
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen+5)+"\n"), nil)
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, maxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, strings.Repeat("a", 10), string(out.Content()))

	// pending message in one raw data
	inChan <- NewPayload([]byte(("1. Hello world!")), nil)
	timeout := time.NewTimer(1*time.Second + 1*time.Millisecond)
	select {
	case out = <-outChan:
		assert.Fail(t, "did not expect message, got ", out)
	case <-timeout.C:
		break
	}

	// decoder context management
	var mockPayloadContext MockPayloadContext

	d.lineBuffer.Reset()
	d.msgBuffer.Reset()

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld\nthisisa"), &mockPayloadContext)
	assert.Equal(t, "helloworld\n", string(mockPayloadContext.Content))
	assert.Equal(t, "", string(mockPayloadContext.Message))

	d.lineBuffer.Reset()
	d.msgBuffer.Reset()

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld\nthisisa\nmessage"), &mockPayloadContext)
	assert.Equal(t, "helloworld\nthisisa\n", string(mockPayloadContext.Content))
	assert.Equal(t, "", string(mockPayloadContext.Message))

	d.lineBuffer.Reset()
	d.msgBuffer.Reset()

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld\nthisisa\n123.message\n"), &mockPayloadContext)
	assert.Equal(t, "helloworld\nthisisa\n", string(mockPayloadContext.Message))
	assert.Equal(t, "123.message\n", string(mockPayloadContext.Content))

	d.lineBuffer.Reset()
	d.msgBuffer.Reset()

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte("helloworld"), &mockPayloadContext)
	assert.Equal(t, "", string(mockPayloadContext.Content))
	assert.Equal(t, "", string(mockPayloadContext.Message))

	d.lineBuffer.Reset()
	d.msgBuffer.Reset()

	mockPayloadContext = MockPayloadContext{}
	d.decodeIncomingData([]byte(strings.Repeat("a", maxMessageLen+5)+"\n"), &mockPayloadContext)
	assert.Equal(t, maxMessageLen, len(mockPayloadContext.Message))
	assert.Equal(t, strings.Repeat("a", 5), string(mockPayloadContext.Content))
}

func TestDecoderLifecycle(t *testing.T) {
	inChan := make(chan *Payload, 10)
	outChan := make(chan message.Message, 10)
	d := New(inChan, outChan, nil)
	d.Start()
	var out message.Message

	inChan <- NewPayload([]byte("helloworld\n"), nil)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))

	d.Stop()
	out = <-outChan
	assert.Equal(t, reflect.TypeOf(out), reflect.TypeOf(message.NewStopMessage()))
}

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

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/stretchr/testify/assert"
)

func TestDecodeIncomingDataForSingleLineLogs(t *testing.T) {
	outChan := make(chan message.Message, 10)
	d := New(nil, outChan, time.Millisecond, nil, true)

	var out message.Message

	// multiple messages in one buffer
	d.decodeIncomingData([]byte(("helloworld\n")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	d.decodeIncomingData([]byte(("helloworld\nhowayou\ngoodandyou")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "howayou", string(out.Content()))
	assert.Equal(t, "goodandyou", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())
	d.lineBuffer.Reset()

	// messages overflow in the next buffer
	d.decodeIncomingData([]byte(("helloworld\nthisisa")), 5)
	assert.Equal(t, "thisisa", d.lineBuffer.String())
	d.decodeIncomingData([]byte(("longinput\nindeed")), 15)
	out = <-outChan
	out = <-outChan
	assert.Equal(t, "thisisalonginput", string(out.Content()))
	assert.Equal(t, "indeed", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// edge cases, do not crash
	d.decodeIncomingData([]byte(("\n\n")), 0)
	d.decodeIncomingData([]byte(("")), 0)

	// buffer overflow
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte(("hello world")), 0)
	d.decodeIncomingData([]byte(("!\n")), 0)
	out = <-outChan
	assert.Equal(t, "hello world!", string(out.Content()))

	// message too big
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte((strings.Repeat("a", config.MaxMessageLen+5) + "\n")), 0)
	out = <-outChan
	assert.Equal(t, config.MaxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, "...TRUNCATED..."+strings.Repeat("a", 20), string(out.Content()))

	// message too big, over several calls
	d.decodeIncomingData([]byte((strings.Repeat("a", config.MaxMessageLen-20))), 0)
	d.decodeIncomingData([]byte((strings.Repeat("a", 25) + "\n")), 0)
	out = <-outChan
	assert.Equal(t, config.MaxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, "...TRUNCATED..."+strings.Repeat("a", 20), string(out.Content()))

	// message too big
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte((strings.Repeat("a", config.MaxMessageLen-15) + "\n")), 0)
	out = <-outChan
	assert.Equal(t, config.MaxMessageLen-15, len(out.Content()))

	// decoder offset management
	d.lineBuffer.Reset()
	d.decodeIncomingData([]byte(("6789\n121416182022\n2527")), 5)
	d.decodeIncomingData([]byte(("29\n")), 27)
	out = <-outChan
	assert.Equal(t, int64(10), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, int64(23), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, int64(30), out.GetOrigin().Offset)
}

func TestDecodeIncomingDataForMultiLineLogs(t *testing.T) {
	inChan := make(chan *Payload, 10)
	outChan := make(chan message.Message, 10)
	re := regexp.MustCompile("[0-9]+\\.")
	singleLine := false
	d := New(inChan, outChan, time.Millisecond, re, singleLine)

	var out message.Message
	go d.run()

	// simple message in one raw data
	inChan <- NewPayload([]byte("1. Hello\nworld!\n"), 5)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple messages in one raw data
	inChan <- NewPayload([]byte("1. Hello\nworld!\n2. How are you\n"), 5)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, int64(36), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// simple message across two raw data
	inChan <- NewPayload([]byte("1. Hello\n"), 5)
	inChan <- NewPayload([]byte("world!\n"), 14)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple messages accross two raw data
	inChan <- NewPayload([]byte("1. Hello\n"), 5)
	inChan <- NewPayload([]byte("world!\n2. How are you\n"), 14)
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, int64(36), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// single-line message in one raw data
	inChan <- NewPayload([]byte("1. Hello world!\n"), 5)
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// multiple single-line messages in one raw data
	inChan <- NewPayload([]byte("1. Hello world!\n2. How are you\n"), 5)
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content()))
	assert.Equal(t, int64(21), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content()))
	assert.Equal(t, int64(36), out.GetOrigin().Offset)
	assert.Equal(t, "", d.lineBuffer.String())
	assert.Equal(t, "", d.msgBuffer.String())

	// big message across two lines
	inChan <- NewPayload([]byte("123456789\n"), 5)
	inChan <- NewPayload([]byte(strings.Repeat("a", maxMessageLen-6)+"\n"), 25)
	out = <-outChan
	assert.Equal(t, config.MaxMessageLen, len(out.Content()))
	out = <-outChan
	assert.Equal(t, "...TRUNCATED..."+strings.Repeat("a", 5), string(out.Content()))

	// pending message in one raw data
	inChan <- NewPayload([]byte(("1. Hello world!")), 5)
	timeout := time.NewTimer(1*time.Second + 1*time.Millisecond)
	select {
	case out = <-outChan:
		assert.Fail(t, "did not expect message, got ", out)
	case <-timeout.C:
		break
	}
}

func TestDecoderLifecycle(t *testing.T) {
	inChan := make(chan *Payload, 10)
	outChan := make(chan message.Message, 10)
	d := New(inChan, outChan, time.Millisecond, nil, true)
	d.Start()
	var out message.Message

	inChan <- NewPayload([]byte(("helloworld\n")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))

	d.Stop()
	out = <-outChan
	assert.Equal(t, reflect.TypeOf(out), reflect.TypeOf(message.NewStopMessage()))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/stretchr/testify/assert"
)

func TestDecodeIncomingData(t *testing.T) {
	outChan := make(chan message.Message, 10)
	d := New(nil, outChan)

	var out message.Message

	// multiple messages in one buffer
	d.decodeIncomingData([]byte(("helloworld\n")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	assert.Equal(t, "", d.msgBuffer.String())
	d.msgBuffer.Reset()

	d.decodeIncomingData([]byte(("helloworld\nhowayou\ngoodandyou")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))
	out = <-outChan
	assert.Equal(t, "howayou", string(out.Content()))
	assert.Equal(t, "goodandyou", d.msgBuffer.String())
	d.msgBuffer.Reset()

	// messages overflow in the next buffer
	d.decodeIncomingData([]byte(("helloworld\nthisisa")), 5)
	assert.Equal(t, "thisisa", d.msgBuffer.String())
	d.decodeIncomingData([]byte(("longinput\nindeed")), 15)
	out = <-outChan
	out = <-outChan
	assert.Equal(t, "thisisalonginput", string(out.Content()))
	assert.Equal(t, "indeed", d.msgBuffer.String())
	d.msgBuffer.Reset()

	// edge cases, do not crash
	d.decodeIncomingData([]byte(("\n\n")), 0)
	d.decodeIncomingData([]byte(("")), 0)

	// buffer overflow
	d.msgBuffer.Reset()
	d.decodeIncomingData([]byte(("hello world")), 0)
	d.decodeIncomingData([]byte(("!\n")), 0)
	out = <-outChan
	assert.Equal(t, "hello world!", string(out.Content()))

	// message too big
	d.msgBuffer.Reset()
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
	d.msgBuffer.Reset()
	d.decodeIncomingData([]byte((strings.Repeat("a", config.MaxMessageLen-15) + "\n")), 0)
	out = <-outChan
	assert.Equal(t, config.MaxMessageLen-15, len(out.Content()))

	// decoder offset management
	d.msgBuffer.Reset()
	d.decodeIncomingData([]byte(("6789\n121416182022\n2527")), 5)
	d.decodeIncomingData([]byte(("29\n")), 27)
	out = <-outChan
	assert.Equal(t, int64(10), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, int64(23), out.GetOrigin().Offset)
	out = <-outChan
	assert.Equal(t, int64(30), out.GetOrigin().Offset)
}

func TestDecoderLifecycle(t *testing.T) {
	inChan := make(chan *Payload, 10)
	outChan := make(chan message.Message, 10)
	d := New(inChan, outChan)
	d.Start()
	var out message.Message

	inChan <- NewPayload([]byte(("helloworld\n")), 0)
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content()))

	d.Stop()
	out = <-outChan
	assert.Equal(t, reflect.TypeOf(out), reflect.TypeOf(message.NewStopMessage()))
}

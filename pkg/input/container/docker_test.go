// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type DockerTailerTestSuite struct {
	suite.Suite
	tailer *DockerTail
}

func (suite *DockerTailerTestSuite) SetupTest() {
	suite.tailer = &DockerTail{}
}

func (suite *DockerTailerTestSuite) TestDockerTailerRemovesDate() {
	msgWithDate := []byte("<2006-01-12T01:01:01.000000000Z my message")
	ts, msg := suite.tailer.parseMessage(msgWithDate)
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(2006, time.January, 12, 1, 1, 1, 0, &time.Location{}).Second(), ts.Second())

	msgWithDate = []byte("g2006-01-12T01:01:01.000000000Z my error")
	ts, msg = suite.tailer.parseMessage(msgWithDate)
	suite.Equal("my error", string(msg))
	suite.Equal(time.Date(2006, time.January, 12, 1, 1, 1, 0, &time.Location{}).Second(), ts.Second())

	sameMsgInBytes := []byte{}
	sameMsgInBytes = append(sameMsgInBytes, '1')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, '0')
	sameMsgInBytes = append(sameMsgInBytes, []byte("<2006-01-12T01:01:01.000000000Z my message")...)
	ts, msg = suite.tailer.parseMessage(sameMsgInBytes)
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(2006, time.January, 12, 1, 1, 1, 0, &time.Location{}).Second(), ts.Second())
}

func TestDockerTailerTestSuite(t *testing.T) {
	suite.Run(t, new(DockerTailerTestSuite))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"fmt"
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
	curYear := time.Now().Year()
	testDate := fmt.Sprintf("%d-01-12T01:01:01.000000000Z", curYear)
	ts, msg := suite.tailer.parseMessage([]byte(fmt.Sprintf("<%s my message", testDate)))
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(curYear, time.January, 12, 1, 1, 1, 0, &time.Location{}).Year(), ts.Year())

	ts, msg = suite.tailer.parseMessage([]byte(fmt.Sprintf("g%s my error", testDate)))
	suite.Equal("my error", string(msg))
	suite.Equal(time.Date(curYear, time.January, 12, 1, 1, 1, 0, &time.Location{}).Year(), ts.Year())

	ts, msg = suite.tailer.parseMessage([]byte(fmt.Sprintf("0%s my message", testDate)))
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(curYear, time.January, 12, 1, 1, 1, 0, &time.Location{}).Year(), ts.Year())

	ts, msg = suite.tailer.parseMessage([]byte(fmt.Sprintf("2%s my message", testDate)))
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(curYear, time.January, 12, 1, 1, 1, 0, &time.Location{}).Year(), ts.Year())

	sameMsgInBytes := []byte{}
	nullBytes := [1]byte{}
	for i := 0; i < 5; i++ {
		sameMsgInBytes = append(sameMsgInBytes, nullBytes[0])
	}
	sameMsgInBytes = append(sameMsgInBytes, []byte(fmt.Sprintf("0%s my message", testDate))...)
	ts, msg = suite.tailer.parseMessage(sameMsgInBytes)
	suite.Equal("my message", string(msg))
	suite.Equal(time.Date(curYear, time.January, 12, 1, 1, 1, 0, &time.Location{}).Year(), ts.Year())
}

func TestDockerTailerTestSuite(t *testing.T) {
	suite.Run(t, new(DockerTailerTestSuite))
}

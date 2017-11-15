// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"testing"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/stretchr/testify/suite"
)

type DockerTailerTestSuite struct {
	suite.Suite
	tailer *DockerTailer
}

func (suite *DockerTailerTestSuite) SetupTest() {
	suite.tailer = &DockerTailer{}
}

func (suite *DockerTailerTestSuite) TestDockerTailerRemovesDate() {
	msgMeta := [8]byte{}
	msgMeta[0] = 1
	// https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
	// next bytes represent the size of the content
	msgMeta[5] = '>'
	msgMeta[6] = '1'
	msgMeta[7] = 'g'

	msg := []byte{}
	for i := 0; i < len(msgMeta); i++ {
		msg = append(msg, msgMeta[i])
	}
	msg = append(msg, []byte("2007-01-12T01:01:01.000000000Z my message")...)
	ts, sev, msg := suite.tailer.parseMessage(msg)
	suite.Equal("my message", string(msg))
	suite.Equal("<46>", string(sev))
	suite.Equal("2007-01-12T01:01:01.000000000Z", ts)

	msgMeta[0] = 2
	msg = []byte{}
	for i := 0; i < len(msgMeta); i++ {
		msg = append(msg, msgMeta[i])
	}
	msg = append(msg, []byte("2008-01-12T01:01:01.000000000Z my error")...)
	ts, sev, msg = suite.tailer.parseMessage(msg)
	suite.Equal("my error", string(msg))
	suite.Equal("<43>", string(sev))
	suite.Equal("2008-01-12T01:01:01.000000000Z", ts)
}

func (suite *DockerTailerTestSuite) TestDockerTailerNextLogSinceDate() {
	suite.Equal("2008-01-12T01:01:01.000000001Z", suite.tailer.nextLogSinceDate("2008-01-12T01:01:01.000000000Z"))
	suite.Equal("2008-01-12T01:01:01.anything", suite.tailer.nextLogSinceDate("2008-01-12T01:01:01.anything"))
	suite.Equal("", suite.tailer.nextLogSinceDate(""))
}

func (suite *DockerTailerTestSuite) TestDockerTailerIdentifier() {
	suite.tailer.containerName = "test"
	suite.Equal("docker:test", suite.tailer.Identifier())
}

func (suite *DockerTailerTestSuite) TestBuildTagsPayload() {
	suite.tailer.containerTags = []string{"test", "hello:world"}
	suite.tailer.source = &config.IntegrationConfigLogSource{Source: "mysource", Tags: "sourceTags"}
	suite.Equal("[dd ddsource=\"mysource\"][dd ddtags=\"test,hello:world,sourceTags\"]", string(suite.tailer.buildTagsPayload()))

	suite.tailer.source = &config.IntegrationConfigLogSource{}
	suite.Equal("[dd ddtags=\"test,hello:world,\"]", string(suite.tailer.buildTagsPayload()))
}

func TestDockerTailerTestSuite(t *testing.T) {
	suite.Run(t, new(DockerTailerTestSuite))
}

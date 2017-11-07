// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PipelineProviderTestSuite struct {
	suite.Suite
	pp *PipelineProvider
}

func (suite *PipelineProviderTestSuite) SetupTest() {
	suite.pp = NewPipelineProvider()
}

func (suite *PipelineProviderTestSuite) TestPipelineProvider() {
	suite.pp.numberOfPipelines = 3
	suite.pp.Start(nil, nil)
	suite.Equal(3, len(suite.pp.pipelinesChans))

	c := suite.pp.NextPipelineChan()
	suite.Equal(1, suite.pp.currentChanIdx)
	suite.pp.NextPipelineChan()
	suite.pp.NextPipelineChan()
	suite.Equal(c, suite.pp.NextPipelineChan())

	suite.pp.MockPipelineChans()
	suite.Equal(1, len(suite.pp.pipelinesChans))
	suite.Equal(1, suite.pp.numberOfPipelines)
}

func TestPipelineProviderTestSuite(t *testing.T) {
	suite.Run(t, new(PipelineProviderTestSuite))
}

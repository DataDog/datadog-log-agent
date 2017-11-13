// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

// Technical constants

const (
	ChanSizes         = 100
	NumberOfPipelines = int32(4)
)

// Business constants

const (
	// MaxMessageLen is the maximum length for any message we send to the intake
	MaxMessageLen = 1 * 1000 * 1000
)

var (
	SEV_INFO  = []byte("<46>")
	SEV_ERROR = []byte("<43>")
)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package sender

import (
	"log"
	"net"
	"strings"
)

// reportNetError submits internal telemetry when we get a network error
func reportNetError(message string, err error) {

	var errMsg string
	switch err := err.(type) {
	case net.Error:
		msgs := strings.Split(err.Error(), ": ")
		errMsg = msgs[len(msgs)-1]
	default:
		errMsg = err.Error()
	}
	log.Println(message, "-", errMsg)
}

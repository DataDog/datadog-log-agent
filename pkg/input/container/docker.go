// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const defaultSleepDuration = 1 * time.Second

// DockerTail tails logs coming from stdout and stderr of a docker container
type DockerTail struct {
	containerName string
	outputChan    chan message.Message
	d             *decoder.Decoder
	source        *config.IntegrationConfigLogSource
	reader        io.ReadCloser
	cli           *client.Client

	sleepDuration time.Duration
}

// NewDockerTailer returns a new DockerTailer
func NewDockerTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, outputChan chan message.Message) *DockerTail {
	return &DockerTail{
		containerName: container.ID,
		outputChan:    outputChan,
		d:             decoder.InitializedDecoder(),
		source:        source,
		cli:           cli,

		sleepDuration: defaultSleepDuration,
	}
}

// Stop stops the DockerTailer
func (dt *DockerTail) Stop(shouldTrackOffset bool) {
	fmt.Println("Stop")
}

// tailFromBegining starts the tailing from the beginning
// of the container logs
func (dt *DockerTail) tailFromBegining() error {
	return dt.tailFrom(time.Time{})
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *DockerTail) tailFromEnd() error {
	return dt.tailFrom(time.Now())
}

// tailFrom starts the tailing from the specified time
func (dt *DockerTail) tailFrom(from time.Time) error {
	dt.d.Start()
	go dt.forwardMessages()
	return dt.startReading(from)
}

// startReading starts the reader that reads the container's stdout,
// with proper configuration
func (dt *DockerTail) startReading(from time.Time) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Since:      from.Format("2006-01-02T15:04:05"),
	}

	reader, err := dt.cli.ContainerLogs(context.Background(), dt.containerName, options)
	if err != nil {
		return err
	}
	dt.reader = reader
	go dt.readForever()
	return nil
}

// readForever reads from the reader as fast as it can,
// and sleeps when there is nothing to read
func (dt *DockerTail) readForever() {
	for {
		inBuf := make([]byte, 4096)
		n, err := dt.reader.Read(inBuf)
		if err == io.EOF {
			dt.wait()
			continue
		}
		if err != nil {
			log.Println("Err:", err)
			return
		}
		if n == 0 {
			dt.wait()
			continue
		}
		dt.d.InputChan <- decoder.NewPayload(inBuf[:n], 0)
	}
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (dt *DockerTail) forwardMessages() {
	for msg := range dt.d.OutputChan {
		_, ok := msg.(*message.StopMessage)
		if ok {
			return
		}

		ts, updatedMsg := dt.parseMessage(msg.Content())

		containerMsg := message.NewContainerMessage(updatedMsg)
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = dt.source
		msgOrigin.Timestamp = &ts
		msgOrigin.Identifier = dt.containerName
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

// parseMessage extracts the date from the raw docker message, which looks like
// <2006-01-12T01:01:01.000000000Z my message
func (dt *DockerTail) parseMessage(msg []byte) (time.Time, []byte) {
	layout := "2006-01-02T15:04:05.000000000Z"
	ts, err := time.Parse(layout, string(msg[1:31]))

	if err != nil {
		fmt.Println(err)
	}
	return ts, msg[32:]
}

// wait lets the reader sleep for a bit
func (dt *DockerTail) wait() {
	time.Sleep(dt.sleepDuration)
}

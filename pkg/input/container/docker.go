// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"

	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const defaultSleepDuration = 1 * time.Second
const Datelayout = "2006-01-02T15:04:05.000000000Z"

// DockerTail tails logs coming from stdout and stderr of a docker container
type DockerTail struct {
	containerName string
	outputChan    chan message.Message
	d             *decoder.Decoder
	source        *config.IntegrationConfigLogSource
	reader        io.ReadCloser
	cli           *client.Client

	sleepDuration time.Duration
	shouldStop    bool
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
func (dt *DockerTail) Stop() {
	dt.shouldStop = true
	dt.d.Stop()
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

		if dt.shouldStop {
			// this means that we stop reading as soon as we get the stop message,
			// but on the other hand we get it when the container is stopped so it should be fine
			return
		}

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

		ts, updatedMsg := dt.updatedDockerMessage(msg.Content())

		containerMsg := message.NewContainerMessage(updatedMsg)
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = dt.source
		msgOrigin.Timestamp = &ts
		msgOrigin.Identifier = dt.containerName
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

func (dt *DockerTail) updatedDockerMessage(msg []byte) (time.Time, []byte) {
	ts, severity, parsedMsg := dt.parseMessage(msg)
	tags, err := tagger.Tag(dockerutil.ContainerIDToEntityName(dt.containerName), false)

	if err != nil {
		log.Println(err)
	}

	updatedMsg := fmt.Sprintf(
		"{\"message\": \"%s\", \"timestamp\": %d, \"ddtags\": \"%s\", \"severity\": \"%s\"}",
		parsedMsg,
		ts.UnixNano()/int64(time.Millisecond),
		strings.Join(tags, ","),
		severity,
	)
	return ts, []byte(updatedMsg)
}

// parseMessage extracts the date from the raw docker message, which looks like
// <2006-01-12T01:01:01.000000000Z my message
func (dt *DockerTail) parseMessage(msg []byte) (time.Time, string, []byte) {
	// Note: We have some null bytes at the beginning of msg,
	// thus looking for the first '<'
	from := bytes.IndexAny(msg, "<g")
	to := bytes.Index(msg, []byte(" "))
	ts, err := time.Parse(Datelayout, string(msg[from+1:to]))

	if err != nil {
		log.Println(err)
		return ts, "", msg
	}
	var severity string
	if msg[from] == '<' { // docker info messages start with '<', while errors start with 'g'
		severity = "info"
	} else {
		severity = "error"
	}
	return ts, severity, msg[to+1:]
}

// wait lets the reader sleep for a bit
func (dt *DockerTail) wait() {
	time.Sleep(dt.sleepDuration)
}

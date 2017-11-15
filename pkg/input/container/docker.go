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

	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const defaultSleepDuration = 1 * time.Second
const DateLayout = "2006-01-02T15:04:05.000000000Z"

// DockerTailer tails logs coming from stdout and stderr of a docker container
// With docker api, there is no way to know if a log comes from strout or stderr
// so if we want to capture the severity, we need to tail both in two goroutines
type DockerTailer struct {
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
func NewDockerTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, outputChan chan message.Message) *DockerTailer {
	return &DockerTailer{
		containerName: container.ID,
		outputChan:    outputChan,
		d:             decoder.InitializeDecoder(source),
		source:        source,
		cli:           cli,

		sleepDuration: defaultSleepDuration,
	}
}

// Identifier returns a string that uniquely identifies a source
func (dt *DockerTailer) Identifier() string {
	return fmt.Sprintf("docker:%s", dt.containerName)
}

// Stop stops the DockerTailer
func (dt *DockerTailer) Stop() {
	dt.shouldStop = true
	dt.d.Stop()
}

// tailFromBegining starts the tailing from the beginning
// of the container logs
func (dt *DockerTailer) tailFromBegining() error {
	return dt.tailFrom(time.Time{}.Format(DateLayout))
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *DockerTailer) tailFromEnd() error {
	return dt.tailFrom(time.Now().UTC().Format(DateLayout))
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we see this container for the first time
func (dt *DockerTailer) recoverTailing(a *auditor.Auditor) error {
	return dt.tailFrom(a.GetLastCommitedTimestamp(dt.Identifier()))
}

// tailFrom starts the tailing from the specified time
func (dt *DockerTailer) tailFrom(from string) error {
	dt.d.Start()
	go dt.forwardMessages()
	return dt.startReading(from)
}

// startReading starts the reader that reads the container's stdout,
// with proper configuration
func (dt *DockerTailer) startReading(from string) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      from,
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
func (dt *DockerTailer) readForever() {
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
		dt.d.InputChan <- decoder.NewPayload(inBuf[:n])
	}
}

// forwardMessages forwards decoded messages to the next pipeline,
// adding a bit of meta information
// Note: For docker container logs, we ask for the timestamp
// to store the time of the last processed line.
// As a result, we need to remove this timestamp from the log
// message before forwarding it
func (dt *DockerTailer) forwardMessages() {
	for msg := range dt.d.OutputChan {
		_, ok := msg.(*message.StopMessage)
		if ok {
			return
		}

		ts, updatedMsg := dt.updatedDockerMessage(msg.Content())

		containerMsg := message.NewContainerMessage(updatedMsg)
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = dt.source
		msgOrigin.Timestamp = ts
		msgOrigin.Identifier = dt.Identifier()
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

func (dt *DockerTailer) updatedDockerMessage(msg []byte) (string, []byte) {
	tags, err := tagger.Tag(dockerutil.ContainerIDToEntityName(dt.containerName), false)
	if err != nil {
		log.Println(err)
	}
	ts, sev, parsedMsg := dt.parseMessage(msg)

	updatedMsg := fmt.Sprintf(
		"{\"message\": %q, \"timestamp\": %q, \"ddtags\": %q, \"severity\": %q}",
		parsedMsg,
		ts,
		strings.Join(tags, ","),
		sev,
	)
	return ts, []byte(updatedMsg)
}

// parseMessage extracts the date and the severity from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func (dt *DockerTailer) parseMessage(msg []byte) (string, string, []byte) {
	// First byte is 1 for stdout and 2 for stderr
	sev := "info"
	if msg[0] == 2 {
		sev = "error"
	}

	// timestamp goes from byte 8 till first space
	from := 8
	to := bytes.Index(msg[from:], []byte{' '})
	if to == -1 {
		log.Println("invalid docker payload collected, skipping message")
		return "", "", msg
	}
	to += from
	ts := string(msg[from:to])
	return ts, sev, msg[to+1:]
}

// wait lets the reader sleep for a bit
func (dt *DockerTailer) wait() {
	time.Sleep(dt.sleepDuration)
}

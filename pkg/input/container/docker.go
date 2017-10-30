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

// recoverTailing starts the tailing from the last log line processed, or now
// if we see this container for the first time
func (dt *DockerTail) recoverTailing(a *auditor.Auditor) error {
	return dt.tailFrom(*c.auditor.GetLastCommitedTimestamp(dt.containerName))
}

// startReading starts the reader that reads the container's stdout,
// with proper configuration
func (dt *DockerTail) startReading(from time.Time) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Details:    false,
		Since:      from.Format("2006-01-02T15:04:05"),
	}
	// FIXME: tail stderr and stdout separately
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
		msgOrigin.Timestamp = ts
		msgOrigin.Identifier = dt.containerName
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

func (dt *DockerTail) updatedDockerMessage(msg []byte) (*time.Time, []byte) {
	ts, parsedMsg := dt.parseMessage(msg)
	tags, err := tagger.Tag(dockerutil.ContainerIDToEntityName(dt.containerName), false)

	if err != nil {
		log.Println(err)
	}

	updatedMsg := fmt.Sprintf(
		"{\"message\": %q, \"timestamp\": %d, \"ddtags\": %q, \"severity\": %q}",
		parsedMsg,
		ts.UnixNano()/int64(time.Millisecond),
		strings.Join(tags, ","),
		"info",
	)
	return ts, []byte(updatedMsg)
}

// parseMessage extracts the date from the raw docker message, which looks like
// XXXXY2006-01-12T01:01:01.000000000Z my message
// X and Y represent some null bytes and a random byte at the beginning of the message
func (dt *DockerTail) parseMessage(msg []byte) (*time.Time, []byte) {
	// We have some random bytes at the beginning of msg, let's skip till we hit the date
	// which starts with a number.
	// If the random byte is a number, we'll get a date far in the past or future, so we can exclude this
	from := bytes.IndexAny(msg, "987654321")
	if from == -1 {
		return nil, msg
	}

	to := bytes.Index(msg[from:], []byte{' '})
	if to == -1 {
		return nil, msg
	}
	to = from + to

	ts, err := time.Parse(Datelayout, string(msg[from:to]))
	correctedTs, err := dt.correctDate(msg[from:to], &ts, err)
	if err != nil {
		log.Println(err)
		return nil, msg
	}
	return correctedTs, msg[to+1:]
}

// Because of the random byte inserted before the date, the date can be wrongly parsed
// if that random byte is a number. When we get a unexpected date, let's retry by dropping the first
// byte, and see if result is relevant
func (dt *DockerTail) correctDate(msg []byte, ts *time.Time, err error) (*time.Time, error) {
	curYear := time.Now().Year()
	computedYear := ts.Year()
	if curYear < computedYear-1 || curYear > computedYear+1 {
		correctedTs, err := time.Parse(Datelayout, string(msg[1:]))
		if err != nil {
			return nil, err
		}
		return &correctedTs, nil
	}
	return ts, err
}

// wait lets the reader sleep for a bit
func (dt *DockerTail) wait() {
	time.Sleep(dt.sleepDuration)
}

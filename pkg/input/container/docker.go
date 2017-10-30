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

// DockerTailer tails logs coming from stdout and stderr of a docker container
// With docker api, there is no way to know if a log comes from strout or stderr
// so if we want to capture the severity, we need to tail both in two goroutines
type DockerTailer struct {
	stdOutTailer *DockerStdTailer
	stdErrTailer *DockerStdTailer
}

// DockerStdTailer tails logs coming from stdout or stderr of a docker container
type DockerStdTailer struct {
	containerName string
	outputChan    chan message.Message
	d             *decoder.Decoder
	source        *config.IntegrationConfigLogSource
	reader        io.ReadCloser
	cli           *client.Client

	shouldTailStdErr bool
	shouldTailStdOut bool
	severity         string

	sleepDuration time.Duration
	shouldStop    bool
}

// NewDockerTailer returns a new DockerTailer
func NewDockerTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, outputChan chan message.Message) *DockerTailer {
	stdOutTailer := DockerStdTailer{
		containerName: container.ID,
		outputChan:    outputChan,
		d:             decoder.InitializedDecoder(),
		source:        source,
		cli:           cli,

		shouldTailStdOut: true,
		severity:         "info",

		sleepDuration: defaultSleepDuration,
	}

	stdErrTailer := DockerStdTailer{
		containerName: container.ID,
		outputChan:    outputChan,
		d:             decoder.InitializedDecoder(),
		source:        source,
		cli:           cli,

		shouldTailStdErr: true,
		severity:         "error",

		sleepDuration: defaultSleepDuration,
	}

	return &DockerTailer{
		stdOutTailer: &stdOutTailer,
		stdErrTailer: &stdErrTailer,
	}
}

// Stop stops the DockerTailer
func (dt *DockerTailer) Stop() {
	dt.stdOutTailer.Stop()
	dt.stdErrTailer.Stop()
}

// tailFromBegining starts the tailing from the beginning
// of the container logs
func (dt *DockerTailer) tailFromBegining() error {
	err := dt.stdOutTailer.tailFrom(time.Time{})
	if err != nil {
		return err
	}
	return dt.stdErrTailer.tailFrom(time.Time{})
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *DockerTailer) tailFromEnd() error {
	err := dt.stdOutTailer.tailFrom(time.Now())
	if err != nil {
		return err
	}
	return dt.stdErrTailer.tailFrom(time.Now())
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we see this container for the first time
func (dt *DockerTailer) recoverTailing(a *auditor.Auditor) error {
	err := dt.stdOutTailer.tailFrom(*a.GetLastCommitedTimestamp(dt.stdOutTailer.originIdentifier()))
	if err != nil {
		return err
	}
	return dt.stdErrTailer.tailFrom(*a.GetLastCommitedTimestamp(dt.stdErrTailer.originIdentifier()))
}

// Stop stops the DockerStdTailer
func (dt *DockerStdTailer) Stop() {
	dt.shouldStop = true
	dt.d.Stop()
}

// tailFromBegining starts the tailing from the beginning
// of the container logs
func (dt *DockerStdTailer) tailFromBegining() error {
	return dt.tailFrom(time.Time{})
}

// tailFromEnd starts the tailing from the last line
// of the container logs
func (dt *DockerStdTailer) tailFromEnd() error {
	return dt.tailFrom(time.Now())
}

// tailFrom starts the tailing from the specified time
func (dt *DockerStdTailer) tailFrom(from time.Time) error {
	dt.d.Start()
	go dt.forwardMessages()
	return dt.startReading(from)
}

// startReading starts the reader that reads the container's stdout,
// with proper configuration
func (dt *DockerStdTailer) startReading(from time.Time) error {
	options := types.ContainerLogsOptions{
		ShowStdout: dt.shouldTailStdOut,
		ShowStderr: dt.shouldTailStdErr,
		Follow:     true,
		Timestamps: true,
		Details:    false,
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
func (dt *DockerStdTailer) readForever() {
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
func (dt *DockerStdTailer) forwardMessages() {
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
		msgOrigin.Identifier = dt.originIdentifier()
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

func (dt *DockerStdTailer) originIdentifier() string {
	return fmt.Sprintf("%s:%s", dt.severity, dt.containerName)
}

func (dt *DockerStdTailer) updatedDockerMessage(msg []byte) (*time.Time, []byte) {
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
		dt.severity,
	)
	return ts, []byte(updatedMsg)
}

// parseMessage extracts the date from the raw docker message, which looks like
// XXXXY2006-01-12T01:01:01.000000000Z my message
// X and Y represent some null bytes and a random byte at the beginning of the message
func (dt *DockerStdTailer) parseMessage(msg []byte) (*time.Time, []byte) {
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
func (dt *DockerStdTailer) correctDate(msg []byte, ts *time.Time, err error) (*time.Time, error) {
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
func (dt *DockerStdTailer) wait() {
	time.Sleep(dt.sleepDuration)
}

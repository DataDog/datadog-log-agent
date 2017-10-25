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

//FIXME comments
type DockerTail struct {
	containerName string
	outputChan    chan message.Message
	d             *decoder.Decoder
	source        *config.IntegrationConfigLogSource
	reader        io.ReadCloser
	cli           *client.Client

	sleepDuration time.Duration
}

// NewDockerTailer(client, container, source, outputChan)
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

func (dt *DockerTail) Stop(shouldTrackOffset bool) {
	fmt.Println("Stop")
}

func (dt *DockerTail) tailFromBegining() error {
	return dt.tailFrom(time.Time{})
}

func (dt *DockerTail) tailFromEnd() error {
	return dt.tailFrom(time.Now())
}

func (dt *DockerTail) tailFrom(from time.Time) error {
	dt.d.Start()
	go dt.forwardMessages()
	return dt.startReading(from)
}

func (dt *DockerTail) startReading(from time.Time) error {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
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

func (dt *DockerTail) forwardMessages() {
	for msg := range dt.d.OutputChan {
		_, ok := msg.(*message.StopMessage)
		if ok {
			return
		}

		// FIXME: drop date?? at least extract
		fmt.Println("Got docker message", string(msg.Content()), "for container", dt.containerName)

		containerMsg := message.NewContainerMessage(msg.Content())
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = dt.source
		ts := time.Now()
		msgOrigin.Timestamp = &ts
		msgOrigin.Identifier = dt.containerName
		containerMsg.SetOrigin(msgOrigin)
		dt.outputChan <- containerMsg
	}
}

// wait lets the tailer sleep for a bit
func (dt *DockerTail) wait() {
	time.Sleep(dt.sleepDuration)
}

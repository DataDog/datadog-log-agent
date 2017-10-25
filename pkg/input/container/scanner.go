// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"context"
	"log"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const scanPeriod = 10 * time.Second

// A ContainerInput listens for stdout and stderr of containers
type ContainerInput struct {
	outputChans [](chan message.Message)
	sources     []*config.IntegrationConfigLogSource
	tailers     map[string]*DockerTail
	cli         *client.Client
	auditor     *auditor.Auditor
}

// New returns an initialized ContainerInput
func New(sources []*config.IntegrationConfigLogSource, outputChans [](chan message.Message), a *auditor.Auditor) *ContainerInput {

	containerSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.DOCKER_TYPE:
			containerSources = append(containerSources, source)
		default:
		}
	}

	return &ContainerInput{
		outputChans: outputChans,
		sources:     containerSources,
		tailers:     make(map[string]*DockerTail),
		auditor:     a,
	}
}

// Start starts the ContainerInput
func (c *ContainerInput) Start() {
	c.setup()
	go c.run()
}

// run lets the ContainerInput tail docker stdouts
func (c *ContainerInput) run() {
	ticker := time.NewTicker(scanPeriod)
	for _ = range ticker.C {
		c.scan()
	}
}

// scan checks for new containers we're expected to
// tail, as well as stopped containers
func (c *ContainerInput) scan() {
	containers := c.listContainers()
	for _, source := range c.sources {
		for _, container := range containers {
			if container.Image == source.Image {
				if _, ok := c.tailers[container.ID]; !ok {
					c.setupTailer(c.cli, container, source, true, c.outputChans[0])
				}
			}
		}
	}
	// FIXME: Stop dead containers if needed
}

func (c *ContainerInput) listContainers() []types.Container {
	containers, err := c.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	return containers
}

// Start starts the ContainerInput
func (c *ContainerInput) setup() {
	if len(c.sources) == 0 {
		return
	}

	// List available containers
	cli, err := client.NewEnvClient()
	c.cli = cli
	if err != nil {
		panic(err)
	}
	containers := c.listContainers()

	// Start tailing monitored containers
	sourceId := 0

	for _, source := range c.sources {
		for _, container := range containers {
			if container.Image == source.Image {
				if _, ok := c.tailers[container.ID]; ok {
					log.Println("Can't tail container twice:", source.Image)
				} else {
					sourceId = (sourceId + 1) % len(c.outputChans)
					c.setupTailer(c.cli, container, source, false, c.outputChans[sourceId%len(c.outputChans)])
				}
			}
		}
	}
}

// setupTailer sets one tailer, making it tail from the begining or the end
func (c *ContainerInput) setupTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, tailFromBegining bool, outputChan chan message.Message) {
	t := NewDockerTailer(cli, container, source, outputChan)
	var err error
	if tailFromBegining {
		err = t.tailFromBegining()
	} else {
		err = t.tailFrom(*c.auditor.GetLastCommitedTimestamp(container.ID))
	}
	if err != nil {
		log.Println(err)
	}
	c.tailers[container.ID] = t
}

// Stop stops the ContainerInput and its tailers
func (c *ContainerInput) Stop() {
	shouldTrackOffset := true
	for _, t := range c.tailers {
		t.Stop(shouldTrackOffset)
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package container

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	dockerutil "github.com/DataDog/datadog-agent/pkg/util/docker"

	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"
)

const scanPeriod = 10 * time.Second
const DOCKER_API_VERSION = "1.25"

// A ContainerInput listens for stdout and stderr of containers
type ContainerInput struct {
	outputChans [](chan message.Message)
	sources     []*config.IntegrationConfigLogSource
	tailers     map[string]*DockerTailer
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
		tailers:     make(map[string]*DockerTailer),
		auditor:     a,
	}
}

// Start starts the ContainerInput
func (c *ContainerInput) Start() {
	err := c.setup()
	if err == nil {
		go c.run()
	}
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
	containersIds := make(map[string]bool)

	// monitor new containers
	for _, source := range c.sources {
		for _, container := range containers {
			if c.sourceShouldMonitorContainer(source, container) {
				containersIds[container.ID] = true
				if _, ok := c.tailers[container.ID]; !ok {
					c.setupTailer(c.cli, container, source, true, c.outputChans[0])
				}
			}
		}
	}

	// stop old containers
	for containerId, tailer := range c.tailers {
		if _, ok := containersIds[containerId]; !ok {
			log.Println("Stop tailing container", containerId[:12])
			tailer.Stop()
			delete(c.tailers, containerId)
		}
	}
}

func (c *ContainerInput) listContainers() []types.Container {
	containers, err := c.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Println("Can't tail containers,", err)
		log.Println("Is datadog-agent part of docker user group?")
		return []types.Container{}
	}
	return containers
}

func (c *ContainerInput) sourceShouldMonitorContainer(source *config.IntegrationConfigLogSource, container types.Container) bool {
	if source.Image != "" && container.Image != source.Image {
		return false
	}
	if source.Label != "" {
		_, ok := container.Labels[source.Label]
		return ok
	}
	return true
}

// Start starts the ContainerInput
func (c *ContainerInput) setup() error {
	if len(c.sources) == 0 {
		return fmt.Errorf("No container source defined")
	}

	// List available containers

	cli, err := client.NewEnvClient()
	// Docker's api updates quickly and is pretty unstable, best pinpoint it
	cli.UpdateClientVersion(DOCKER_API_VERSION)
	c.cli = cli
	if err != nil {
		log.Println("Can't tail containers,", err)
		return fmt.Errorf("Can't initialize client")
	}
	containers := c.listContainers()

	// Initialize docker utils
	err = tagger.Init()
	if err != nil {
		log.Println(err)
	}
	dockerutil.InitDockerUtil(&dockerutil.Config{
		CacheDuration:  10 * time.Second,
		CollectNetwork: false,
	})

	// Start tailing monitored containers
	sourceId := 0

	for _, source := range c.sources {
		for _, container := range containers {
			if c.sourceShouldMonitorContainer(source, container) {
				if _, ok := c.tailers[container.ID]; ok {
					log.Println("Can't tail container twice:", source.Image)
				} else {
					sourceId = (sourceId + 1) % len(c.outputChans)
					c.setupTailer(c.cli, container, source, false, c.outputChans[sourceId%len(c.outputChans)])
				}
			}
		}
	}
	return nil
}

// setupTailer sets one tailer, making it tail from the begining or the end
func (c *ContainerInput) setupTailer(cli *client.Client, container types.Container, source *config.IntegrationConfigLogSource, tailFromBegining bool, outputChan chan message.Message) {
	log.Println("Detected container", container.Image, "-", container.ID[:12])
	t := NewDockerTailer(cli, container, source, outputChan)
	var err error
	if tailFromBegining {
		err = t.tailFromBegining()
	} else {
		err = t.recoverTailing(c.auditor)
	}
	if err != nil {
		log.Println(err)
	}
	c.tailers[container.ID] = t
}

// Stop stops the ContainerInput and its tailers
func (c *ContainerInput) Stop() {
	for _, t := range c.tailers {
		t.Stop()
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tailer

import (
	"log"
	"os"
	"syscall"
	"time"

	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

const scanPeriod = 10 * time.Second

type Scanner struct {
	sources     []*config.IntegrationConfigLogSource
	outputChans [](chan message.Message)
	tailers     map[string]*Tailer
	auditor     *auditor.Auditor
}

// New returns an initialized Scanner
func New(sources []*config.IntegrationConfigLogSource, outputChans [](chan message.Message), auditor *auditor.Auditor) *Scanner {
	tailSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.FILE_TYPE:
			tailSources = append(tailSources, source)
		default:
		}
	}
	return &Scanner{
		sources:     tailSources,
		outputChans: outputChans,
		tailers:     make(map[string]*Tailer),
		auditor:     auditor,
	}
}

// setup sets all tailers
func (s *Scanner) setup() {
	for sourceId, source := range s.sources {
		if _, ok := s.tailers[source.Path]; ok {
			log.Println("Can't tail file twice:", source.Path)
		} else {
			s.setupTailer(source, false, s.outputChans[sourceId%len(s.outputChans)])
		}
	}
}

// setupTailer sets one tailer, making it tail from the begining or the end
func (s *Scanner) setupTailer(source *config.IntegrationConfigLogSource, tailFromBegining bool, outputChan chan message.Message) {
	t := NewTailer(outputChan, source)
	var err error
	if tailFromBegining {
		err = t.tailFromBegining()
	} else {
		// resume tailing from last commited offset
		err = t.tailFrom(s.auditor.GetLastCommitedOffset(t.source))
	}
	if err != nil {
		log.Println(err)
	}
	s.tailers[source.Path] = t
}

// Start starts the Scanner
func (s *Scanner) Start() {
	s.setup()
	go s.run()
}

// run lets the Scanner tail its file
func (s *Scanner) run() {
	ticker := time.NewTicker(scanPeriod)
	for _ = range ticker.C {
		s.scan()
	}
}

// scan checks all the files we're expected to tail,
// compares them to the currently tailed files,
// and triggeres the required updates.
// For instance, when a file is logrotated,
// its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer,
// and start a new one for the new file.
func (s *Scanner) scan() {
	for _, source := range s.sources {
		tailer := s.tailers[source.Path]
		f, err := os.Open(source.Path)
		if err != nil {
			continue
		}
		stat1, err := f.Stat()
		if err != nil {
			continue
		}
		stat2, err := tailer.file.Stat()
		if err != nil {
			s.onFileRotation(tailer, source)
			continue
		}
		if inode(stat1) != inode(stat2) {
			s.onFileRotation(tailer, source)
			continue
		}

		if stat1.Size() < tailer.GetLastOffset() {
			tailer.reset()
		}
	}
}

func (s *Scanner) onFileRotation(tailer *Tailer, source *config.IntegrationConfigLogSource) {
	shouldTrackOffset := false
	tailer.Stop(shouldTrackOffset)
	s.setupTailer(source, true, tailer.outputChan)
}

// Stop stops the Scanner and its tailers
func (s *Scanner) Stop() {
	shouldTrackOffset := true
	for _, t := range s.tailers {
		t.Stop(shouldTrackOffset)
	}
}

// inode uniquely identifies a file on a filesystem
func inode(f os.FileInfo) uint64 {
	s := f.Sys()
	if s == nil {
		return 0
	}
	switch s := s.(type) {
	case *syscall.Stat_t:
		return uint64(s.Ino)
	default:
		return 0
	}
}

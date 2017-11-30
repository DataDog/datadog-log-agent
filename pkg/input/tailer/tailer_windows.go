// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build windows

package tailer

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-log-agent/pkg/auditor"
	"github.com/DataDog/datadog-log-agent/pkg/config"
	"github.com/DataDog/datadog-log-agent/pkg/decoder"
	"github.com/DataDog/datadog-log-agent/pkg/message"
)

const defaultSleepDuration = 1 * time.Second
const defaultCloseTimeout = 60 * time.Second

type dirwatch struct {
	ov  syscall.Overlapped
	buf [4096]byte
}
type Tailer struct {
	path     string
	fullpath string
	dirpath  string

	lastOffset        int64
	whence            int
	shouldTrackOffset bool

	outputChan chan message.Message
	d          *decoder.Decoder
	source     *config.IntegrationConfigLogSource

	closeTimeout time.Duration
	shouldStop   bool
	stopTimer    *time.Timer
	stopMutex    sync.Mutex

	dirHandle syscall.Handle
	iocp      syscall.Handle
}

// NewTailer returns an initialized Tailer
func NewTailer(outputChan chan message.Message, source *config.IntegrationConfigLogSource) *Tailer {
	return &Tailer{
		path:       source.Path,
		outputChan: outputChan,
		d:          decoder.InitializedDecoder(),
		source:     source,

		lastOffset:        0,
		whence:            io.SeekStart,
		shouldTrackOffset: true,

		shouldStop: false,
		stopMutex:  sync.Mutex{},
		//closeTimeout:  defaultCloseTimeout,
	}
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return fmt.Sprintf("file:%s", t.source.Path)
}

// recoverTailing starts the tailing from the last log line processed, or now
// if we tail this file for the first time
func (t *Tailer) recoverTailing(a *auditor.Auditor) error {
	return t.tailFrom(a.GetLastCommitedOffset(t.Identifier()))
}

// Stop lets  the tailer stop
func (t *Tailer) Stop(shouldTrackOffset bool) {
	t.stopMutex.Lock()
	t.shouldStop = true
	t.shouldTrackOffset = shouldTrackOffset
	t.stopTimer = time.NewTimer(t.closeTimeout)
	t.stopMutex.Unlock()
	syscall.PostQueuedCompletionStatus(t.iocp, 0, 1, nil)
}

// onStop handles the housekeeping when we stop the tailer
func (t *Tailer) onStop() {
	t.stopMutex.Lock()
	t.d.Stop()
	log.Println("Closing", t.path)
	t.stopTimer.Stop()
	t.stopMutex.Unlock()
}

// tailFrom let's the tailer open a file and tail from whence
func (t *Tailer) tailFrom(offset int64, whence int) error {
	t.d.Start()
	err := t.startReading(offset, whence)
	if err == nil {
		go t.forwardMessages()
	}
	return err
}

func (t *Tailer) startReading(offset int64, whence int) error {
	var err error
	t.fullpath, err = filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.dirpath = filepath.Dir(t.fullpath)

	t.dirHandle, err = syscall.CreateFile(syscall.StringToUTF16Ptr(t.dirpath),
		syscall.FILE_LIST_DIRECTORY,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OVERLAPPED,
		0)
	if err != nil {
		return err
	}
	t.lastOffset = offset
	t.whence = whence
	t.iocp, err = syscall.CreateIoCompletionPort(t.dirHandle, 0, 0, 0)
	if err != nil {
		return err
	}
	go t.readForever()
	return nil
}

// tailFromBegining lets the tailer start tailing its file
// from the begining
func (t *Tailer) tailFromBegining() error {
	return t.tailFrom(0, os.SEEK_SET)
}

// tailFromEnd lets the tailer start tailing its file
// from the end
func (t *Tailer) tailFromEnd() error {
	return t.tailFrom(0, os.SEEK_END)
}

// reset makes the tailer seek the begining of its file
func (t *Tailer) reset() {
	t.setLastOffset(0)
}

// forwardMessages lets the Tailer forward log messages to the output channel
func (t *Tailer) forwardMessages() {
	for msg := range t.d.OutputChan {

		_, ok := msg.(*message.StopMessage)
		if ok {
			return
		}

		fileMsg := message.NewFileMessage(msg.Content())
		msgOffset := msg.GetOrigin().Offset
		identifier := t.Identifier()
		if !t.shouldTrackOffset {
			msgOffset = 0
			identifier = ""
		}
		msgOrigin := message.NewOrigin()
		msgOrigin.LogSource = t.source
		msgOrigin.Identifier = identifier
		msgOrigin.Offset = msgOffset
		fileMsg.SetOrigin(msgOrigin)
		t.outputChan <- fileMsg
	}
}

func (t *Tailer) readAvailable() (bool, error) {

	inBuf := make([]byte, 4096)
	f, err := os.Open(t.fullpath)
	if err != nil {
		return true, err
	}
	defer f.Close()
	f.Seek(t.lastOffset, io.SeekStart)
	for {
		n, err := f.Read(inBuf)
		if n == 0 || err != nil {
			break
		}
		t.d.InputChan <- decoder.NewPayload(inBuf[:n], t.GetLastOffset())
		t.incrementLastOffset(n)
	}
	if t.shouldSoftStop() {
		t.onStop()
		return true, nil
	}
	return false, nil

}

// readForever lets the tailer tail the content of a file
// until it is closed.
func (t *Tailer) readForever() {

	if ret, _ := t.readAvailable(); ret {
		return
	}

	var directory dirwatch
	var mask uint32
	mask = syscall.FILE_NOTIFY_CHANGE_LAST_WRITE |
		syscall.FILE_NOTIFY_CHANGE_FILE_NAME |
		syscall.FILE_NOTIFY_CHANGE_CREATION

	for {
		err := syscall.ReadDirectoryChanges(t.dirHandle, &directory.buf[0],
			uint32(unsafe.Sizeof(directory.buf)), false, mask, nil, &directory.ov, 0)

		if err != nil {
			log.Fatalf("Error ReadDirectoryChanges: %s\n", err.Error())
			return
		}

		var n, key uint32
		var ol *syscall.Overlapped
		err = syscall.GetQueuedCompletionStatus(t.iocp, &n, &key, &ol, syscall.INFINITE)
		// Point "raw" to the event in the buffer
		var offset uint32
		if key != 0 {
			log.Printf("Got stop key, stopping\n")
			return
		}
		for {
			raw := (*syscall.FileNotifyInformation)(unsafe.Pointer(&directory.buf[offset]))
			buf := (*[syscall.MAX_PATH]uint16)(unsafe.Pointer(&raw.FileName))
			name := syscall.UTF16ToString(buf[:raw.FileNameLength/2])

			changename := filepath.Join(t.dirpath, name)
			if changename == t.path {
				switch raw.Action {
				case syscall.FILE_ACTION_ADDED:
					log.Printf("matching file added %s\n", name)
					// reset offset to zero
					t.setLastOffset(0)
					if ret, _ := t.readAvailable(); ret {
						return
					}

				case syscall.FILE_ACTION_REMOVED:
					log.Printf("matching file removed %s\n", name)
					// it's OK that it was removed; just set the read index
					// back to zero
					t.setLastOffset(0)
				case syscall.FILE_ACTION_MODIFIED:
					log.Printf("matching file modified %s\n", name)
					if ret, _ := t.readAvailable(); ret {
						return
					}

				case syscall.FILE_ACTION_RENAMED_OLD_NAME:
					// was renamed to a different file (rotated?).  Set the
					// read index back to zero
					log.Printf("matching file renamed from %s\n", name)
					t.lastOffset = 0

				case syscall.FILE_ACTION_RENAMED_NEW_NAME:
					log.Printf("matching file renamed to %s\n", name)
					// file was renamed into the file we're watching.  Start
					// from zero? start from current position?
					t.setLastOffset(0)
					if ret, _ := t.readAvailable(); ret {
						return
					}

				}
			}

			if raw.NextEntryOffset == 0 {
				break
			}
			offset += raw.NextEntryOffset
		}

	}

}

func (t *Tailer) checkForRotation() bool {
	return false
}
func (t *Tailer) shouldHardStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	if t.stopTimer != nil {
		select {
		case <-t.stopTimer.C:
			return true
		default:
		}
	}
	return false
}

func (t *Tailer) shouldSoftStop() bool {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	return t.shouldStop
}

func (t *Tailer) incrementLastOffset(n int) {
	atomic.AddInt64(&t.lastOffset, int64(n))
}

func (t *Tailer) setLastOffset(n int64) {
	atomic.StoreInt64(&t.lastOffset, n)
}

func (t *Tailer) GetLastOffset() int64 {
	return atomic.LoadInt64(&t.lastOffset)
}

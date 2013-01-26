/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"bufio"
	"errors"
	"github.com/howeyc/fsnotify"
	"log"
	"os"
	"sync"
	"time"
)

type LogfileInputConfig struct {
	SincedbFlush int
	LogFiles     []string
}

type LogfileInput struct {
	Monitor *FileMonitor
	logline Logline
}

type Logline struct {
	Path string
	Line string
}

func (lw *LogfileInput) ConfigStruct() interface{} {
	return &LogfileInputConfig{SincedbFlush: 1}
}

func (lw *LogfileInput) Init(config interface{}) (err error) {
	conf := config.(*LogfileInputConfig)
	lw.Monitor = new(FileMonitor)
	if err = lw.Monitor.Init(conf.LogFiles); err != nil {
		return err
	}
	return nil
}

func (lw *LogfileInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	ticker := time.NewTicker(*timeout)
	select {
	case <-ticker.C:
		return errors.New("Timeout waiting for log line")
	case lw.logline = <-lw.Monitor.NewLines:
		pipelinePack.Message.Type = "logfile"
		pipelinePack.Message.Payload = lw.logline.Line
		pipelinePack.Message.Logger = lw.logline.Path
		pipelinePack.Decoded = true
	}
	return nil
}

func (lw *LogfileInput) Event(eventType string) {
	lw.Monitor.Event(eventType)
}

// FileMonitor, manages a group of FileTailers
//
// The FileMonitor
type FileMonitor struct {
	watcher   *fsnotify.Watcher
	events    chan string
	NewLines  chan Logline
	waitgroup *sync.WaitGroup
	seek      map[string]int64
	discover  map[string]bool
	fds       map[string]*os.File
}

func (fm *FileMonitor) Watcher(initialFiles []string) {
	var ev *fsnotify.FileEvent
	discovery := time.NewTicker(time.Second * 5)

	// Attempt for files that should exist, to start reading them at
	// the end
	for _, fileName := range initialFiles {
		fm.ReadLines(fileName, nil)
	}

	for {
		select {
		case ev = <-fm.watcher.Event:
			fm.ReadLines(ev.Name, ev)
		case <-fm.watcher.Error:
			continue
		case <-discovery.C:
			// Check to see if the files exist now, start reading them
			// if we can, and watch them
			for fileName, needsWatch := range fm.discover {
				if !needsWatch {
					continue
				}
				err := fm.watcher.Watch(fileName)
				if err != nil {
					delete(fm.discover, fileName)
					fm.ReadLines(fileName, nil)
				}
			}
		}
	}
}

func (fm *FileMonitor) ReadLines(fileName string, event *fsnotify.FileEvent) {
	// First see if we have an fd for this file already
	var seek int64
	var fd *os.File
	var found bool
	var err error
	if fd, found = fm.fds[fileName]; !found {
		// Attempt to open the file
		fd, err = os.Open(fileName)
		if err != nil {
			// No such file, put it back on discover
			fm.discover[fileName] = true
			fm.watcher.RemoveWatch(fileName)
			return
		}
		fm.fds[fileName] = fd

		// Should we seek?
		offset := int64(0)
		begin := 0
		if seek, found = fm.seek[fileName]; found {
			offset = seek
			begin = 0
		}
		fm.seek[fileName] = offset
		_, err = fd.Seek(offset, begin)
		if err != nil {
			// Unable to seek in, start at beginning
			fd.Seek(0, 0)
			fm.seek[fileName] = 0
		}
	}

	// Attempt to read lines from where we are
	reader := bufio.NewReader(fd)
	readLine, err := reader.ReadString('\n')
	for err == nil {
		line := Logline{Path: fileName, Line: readLine}
		fm.seek[fileName] += int64(len(readLine))
		fm.NewLines <- line
		readLine, err = reader.ReadString('\n')
	}
	fm.seek[fileName] += int64(len(readLine))

	// Does our file still exist?
	if event != nil {
		if event.IsDelete() || event.IsRename() {
			delete(fm.fds, fileName)
			fd.Close()
			fm.watcher.RemoveWatch(fileName)
			fm.discover[fileName] = true
			fm.seek[fileName] = 0
		}
	}

	return
}

func (fm *FileMonitor) Init(files []string) (err error) {
	fm.waitgroup = new(sync.WaitGroup)
	fm.events = make(chan string, 1)
	fm.NewLines = make(chan Logline, 1500)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)
	fm.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return
	}

	// Add all the files to watch
	initialFiles := make([]string, 0, len(files))

	for _, fileName := range files {
		err = fm.watcher.Watch(fileName)
		if err != nil {
			// Error watching a file, warn and save it to discover
			fm.discover[fileName] = true
			log.Printf("Unable to find '%s' file, adding to discover list.",
				fileName)
		} else {
			initialFiles = append(initialFiles, fileName)
			log.Printf("Found %s, added to seek", fileName)
		}
		err = nil
	}

	// Launch the file reader
	go fm.Watcher(initialFiles)

	return
}

// Respond to an event
//
// If its a STOP event, wait until the Watcher goroutine shuts down
// gracefully
func (fm *FileMonitor) Event(eventType string) {
	fm.events <- eventType
	if eventType == STOP {
		fm.waitgroup.Wait()
	}
}

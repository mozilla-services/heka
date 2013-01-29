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
	select {
	case <-time.After(*timeout):
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
	events    chan string
	NewLines  chan Logline
	waitgroup *sync.WaitGroup
	seek      map[string]int64
	discover  map[string]bool
	fds       map[string]*os.File
}

func (fm *FileMonitor) OpenFile(fileName string) (err error) {
	// Attempt to open the file
	fd, err := os.Open(fileName)
	if err != nil {
		return
	}
	fm.fds[fileName] = fd

	// Should we seek?
	offset := int64(0)
	begin := 0
	seek, found := fm.seek[fileName]
	if found {
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
	return nil
}

func (fm *FileMonitor) Watcher() {
	discovery := time.NewTicker(time.Second * 5)
	checkStat := time.NewTicker(time.Millisecond * 500)

	for {
		select {
		case <-checkStat.C:
			for fileName, _ := range fm.fds {
				fm.ReadLines(fileName)
			}
		case <-discovery.C:
			// Check to see if the files exist now, start reading them
			// if we can, and watch them
			for fileName, _ := range fm.discover {
				err := fm.OpenFile(fileName)
				if err != nil {
					delete(fm.discover, fileName)
				}
			}
		}
	}
}

func (fm *FileMonitor) ReadLines(fileName string) {
	fd, _ := fm.fds[fileName]

	// Determine if we're farther into the file than possible (truncate)
	finfo, err := fd.Stat()
	if err == nil {
		if finfo.Size() < fm.seek[fileName] {
			fd.Seek(0, 0)
			fm.seek[fileName] = 0
		}
	} else {
		// Got an error, move this to discovery, reset seek
		fd.Close()
		delete(fm.fds, fileName)
		delete(fm.seek, fileName)
		fm.discover[fileName] = true
		return
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
	return
}

func (fm *FileMonitor) Init(files []string) (err error) {
	fm.waitgroup = new(sync.WaitGroup)
	fm.events = make(chan string, 1)
	fm.NewLines = make(chan Logline)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)
	if err != nil {
		return
	}

	for _, fileName := range files {
		err = fm.OpenFile(fileName)
		if err != nil {
			// No such file, keep it on discover
			fm.discover[fileName] = true
		}
		err = nil
	}

	// Launch the file reader
	go fm.Watcher()

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

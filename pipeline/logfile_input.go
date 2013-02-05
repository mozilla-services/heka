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
	"fmt"
	"os"
	"time"
)

type LogfileInputConfig struct {
	SincedbFlush int
	LogFiles     [][]string
}

type LogfileInput struct {
	Monitor    *FileMonitor
	DecoderMap map[string][]string
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
	lw.DecoderMap = make(map[string][]string)
	if err = lw.Monitor.Init(conf.LogFiles); err != nil {
		return err
	}
	for _, logconf := range conf.LogFiles {
		if len(logconf) > 1 {
			copy(lw.DecoderMap[logconf[0]], logconf[1:])
		}
	}
	return nil
}

func (lw *LogfileInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) (err error) {
	select {
	case <-time.After(*timeout):
		return errors.New("Timeout waiting for log line")
	case logline := <-lw.Monitor.NewLines:
		pipelinePack.Message.SetType("logfile")
		pipelinePack.Message.SetPayload(logline.Line)
		pipelinePack.Message.SetLogger(logline.Path)
		pipelinePack.Decoded = true

		decoderNames, ok := lw.DecoderMap[logline.Path]
		if !ok {
			return nil
		}
		for _, decoderName := range decoderNames {
			decoder, ok := pipelinePack.Decoders[decoderName]
			if !ok {
				return fmt.Errorf("Unable to find configured decoder for log line %s",
					logline.Path)
			}
			err = decoder.Decode(pipelinePack)
			if err == nil {
				return
			}
		}
	}
	return
}

func (lw *LogfileInput) Event(eventType string) {
	lw.Monitor.Event(eventType)
}

// FileMonitor, manages a group of FileTailers
//
// The FileMonitor
type FileMonitor struct {
	NewLines chan Logline
	seek     map[string]int64
	discover map[string]bool
	fds      map[string]*os.File
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
	seek, ok := fm.seek[fileName]
	if ok {
		offset = seek
	} else {
		fm.seek[fileName] = offset
	}
	_, err = fd.Seek(offset, begin)
	if err != nil {
		// Unable to seek in, start at beginning
		if _, err = fd.Seek(0, 0); err != nil {
			return
		}
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
				if fm.OpenFile(fileName) == nil {
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

	// Check that we haven't been rotated, if we have, put this back on
	// discover
	pinfo, err := os.Stat(fileName)
	if err != nil || !os.SameFile(pinfo, finfo) {
		fd.Close()
		delete(fm.fds, fileName)
		delete(fm.seek, fileName)
		fm.discover[fileName] = true
	}
	return
}

func (fm *FileMonitor) Init(files [][]string) (err error) {
	fm.NewLines = make(chan Logline)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)
	for _, fileData := range files {
		fileName := fileData[0]
		fm.discover[fileName] = true
	}
	go fm.Watcher()
	return
}

// Respond to an event
//
// If its a STOP event, wait until the Watcher goroutine shuts down
// gracefully
func (fm *FileMonitor) Event(eventType string) {
}

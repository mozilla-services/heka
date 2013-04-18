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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bufio"
	"log"
	"os"
	"time"
)

type LogfileInputConfig struct {
	SincedbFlush int
	LogFiles     []string
	Hostname     string
}

type LogfileInput struct {
	Monitor  *FileMonitor
	hostname string
	stopped  bool
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
	val := conf.Hostname
	if val == "" {
		val, err = os.Hostname()
		if err != nil {
			return
		}
	}
	lw.hostname = val
	if err = lw.Monitor.Init(conf.LogFiles); err != nil {
		return err
	}
	return nil
}

func (lw *LogfileInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var pack *PipelinePack
	packSupply := ir.InChan()

	for logline := range lw.Monitor.NewLines {
		pack = <-packSupply
		pack.Message.SetType("logfile")
		pack.Message.SetPayload(logline.Line)
		pack.Message.SetLogger(logline.Path)
		pack.Message.SetHostname(lw.hostname)
		pack.Decoded = true
		ir.Inject(pack)
	}

	log.Println("Input stopped: LogfileInput")
	return
}

func (lw *LogfileInput) Stop() {
	close(lw.Monitor.stopChan) // stops the monitor's watcher
}

// FileMonitor, manages a group of FileTailers
//
// The FileMonitor
type FileMonitor struct {
	NewLines  chan Logline
	stopChan  chan bool
	seek      map[string]int64
	discover  map[string]bool
	fds       map[string]*os.File
	checkStat <-chan time.Time
}

func (fm *FileMonitor) OpenFile(fileName string) (err error) {
	// Attempt to open the file
	fd, err := os.Open(fileName)
	if err != nil {
		return
	}
	fm.fds[fileName] = fd

	// Seek as needed
	begin := 0
	offset := fm.seek[fileName]
	_, err = fd.Seek(offset, begin)
	if err != nil {
		// Unable to seek in, start at beginning
		fm.seek[fileName] = 0
		if _, err = fd.Seek(0, 0); err != nil {
			return
		}
	}
	return nil
}

func (fm *FileMonitor) Watcher() {
	discovery := time.Tick(time.Second * 5)
	checkStat := time.Tick(time.Millisecond * 500)

	for {
		select {
		case <-checkStat:
			for fileName, _ := range fm.fds {
				fm.ReadLines(fileName)
			}
		case <-discovery:
			// Check to see if the files exist now, start reading them
			// if we can, and watch them
			for fileName, _ := range fm.discover {
				if fm.OpenFile(fileName) == nil {
					delete(fm.discover, fileName)
				}
			}
		case <-fm.stopChan:
			for _, fd := range fm.fds {
				fd.Close()
			}
			close(fm.NewLines)
			return
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

func (fm *FileMonitor) Init(files []string) (err error) {
	fm.NewLines = make(chan Logline)
	fm.stopChan = make(chan bool)
	fm.seek = make(map[string]int64)
	fm.fds = make(map[string]*os.File)
	fm.discover = make(map[string]bool)
	for _, fileName := range files {
		fm.discover[fileName] = true
	}
	go fm.Watcher()
	return
}

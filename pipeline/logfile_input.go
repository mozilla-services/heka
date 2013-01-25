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
	"github.com/howeyc/fsnotify"
	"sync"
)

type LogfileInputConfig struct {
	SincedbFlush int
	LogFiles     []string
}

type LogfileInput struct {
	Monitor *FileMonitor
}

type Logline struct {
	Path string
	Line []byte
}

func (lw *LogfileWriter) ConfigStruct() interface{} {
	return &LogfileInputConfig{5}
}

func (lw *LogfileInput) Init(config interface{}) error {
	conf := config.(*LogfileInputConfig)
	lw.Monitor = new(FileMonitor)
	if err := lw.Monitor.Init(conf.LogFiles); err != nil {
		return err
	}
	return nil
}

func (lw *LogfileInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) (err error) {

}

func (lw *LogfileInput) Event(eventType string) {

}

// FileMonitor, manages a group of FileTailers
//
// The FileMonitor
type FileMonitor struct {
	watcher   *fsnotify.Watcher
	events    <-chan string
	NewLines  chan *Logline
	waitgroup *sync.WaitGroup
	Sincedb   map[string]int64
}

func (fm *FileMonitor) Watcher() {
	fm.waitgroup.Add(1)
	var event string
	var ev *fsnotify.FileEvent

	for {
		select {
		case event = <-fm.events:
			if event == STOP {
				break
			}
		case ev = <-fm.watcher.Event:
			logline := make([]byte, 800)
			line := new(Logline)
			line.Path = ev.Name
			line.Line = logline
			fm.NewLines <- line
		case fileErr := <-fm.watcher.Error:
			nil
		}
	}
	fm.waitgroup.Done()
}

func (fm *FileMonitor) Init(files []string, events <-chan string) (err error) {
	fm.waitgroup = new(sync.WaitGroup)
	fm.events = events
	fm.OutLines = make(chan *Logline)
	fm.NewLines = make(chan *Logline, 1)
	line := new(Logline)
	line.Path = make([]byte, 500)
	fm.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return
	}

	// Launch the file reader
	go fm.Watcher()

	// Add all the files to watch
	for fileName := range files {
		err = fm.watcher.Watch(fileName)
		if err != nil {
			break
		}
	}
	return
}

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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/rafrombrc/go-notify"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Output interface {
	Deliver(pipelinePack *PipelinePack)
}

type LogOutput struct {
}

func (self *LogOutput) Init(config interface{}) error {
	return nil
}

func (self *LogOutput) Deliver(pipelinePack *PipelinePack) {
	log.Printf("%+v\n", *(pipelinePack.Message))
}

type CounterOutput struct {
}

type CounterGlobal struct {
	count chan uint
	once  sync.Once
}

var counterGlobal CounterGlobal

func InitCountChan() {
	counterGlobal.count = make(chan uint, 30000)
	go counterLoop()
}

func (self *CounterOutput) Init(config interface{}) error {
	counterGlobal.once.Do(InitCountChan)
	return nil
}

func (self *CounterOutput) Deliver(pipelinePack *PipelinePack) {
	counterGlobal.count <- 1
}

func counterLoop() {
	tick := time.NewTicker(time.Duration(time.Second))
	aggregate := time.NewTicker(time.Duration(10 * time.Second))
	lastTime := time.Now()
	lastCount := uint(0)
	count := uint(0)
	zeroes := int8(0)
	var (
		msgsSent, inc uint
		elapsedTime   time.Duration
		now           time.Time
		rate          float64
		rates         []float64
	)
	for {
		// Here for performance reasons
		runtime.Gosched()
		select {
		case <-aggregate.C:
			amount := len(rates)
			if amount < 1 {
				continue
			}
			sort.Float64s(rates)
			min := rates[0]
			max := rates[amount-1]
			mean := min
			sum := float64(0)
			for _, val := range rates {
				sum += val
			}
			mean = sum / float64(amount)
			log.Printf("AGG Sum. Min: %0.2f   Max: %0.2f     Mean: %0.2f",
				min, max, mean)
			rates = rates[:0]
		case <-tick.C:
			now = time.Now()
			msgsSent = count - lastCount
			lastCount = count
			elapsedTime = now.Sub(lastTime)
			lastTime = now
			rate = float64(msgsSent) / elapsedTime.Seconds()
			if msgsSent == 0 {
				if msgsSent == 0 || zeroes == 3 {
					continue
				}
				zeroes++
			} else {
				zeroes = 0
			}
			log.Printf("Got %d messages. %0.2f msg/sec\n", count, rate)
			rates = append(rates, rate)
		case inc = <-counterGlobal.count:
			count += inc
		}
	}
}

type FileOutputWriter struct {
	path        string
	file        *os.File
	outputBytes []byte
	ticker      *time.Ticker
}

func NewFileOutputWriter(path string, perm os.FileMode) (*FileOutputWriter,
	error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, perm)
	if err != nil {
		return nil, err
	}
	self := &FileOutputWriter{path: path, file: file}
	go self.fileSyncer()
	return self, nil
}

func (self *FileOutputWriter) fileSyncer() {
	self.ticker = time.NewTicker(time.Second)
	for _ = range self.ticker.C {
		self.file.Sync()
	}
}

func (self *FileOutputWriter) MakeOutputData() interface{} {
	return make([]byte, 0, 2000)
}

func (self *FileOutputWriter) Write(outputData interface{}) (err error) {
	self.outputBytes = outputData.([]byte)
	n, err := self.file.Write(self.outputBytes)
	if err != nil {
		err = fmt.Errorf("FileOutput error writing to %s: %s", self.path, err)
	} else if n != len(self.outputBytes) {
		err = fmt.Errorf("FileOutput truncated output for %s", self.path)
	}
	self.outputBytes = self.outputBytes[:0]
	return err
}

func (self *FileOutputWriter) Stop() {
	self.ticker.Stop()
	self.file.Close()
}

var (
	FileDataRecyclers = make(map[string]DataRecycler)

	FILEFORMATS = map[string]bool{
		"json": true,
		"text": true,
	}

	TSFORMAT = "[2006/Jan/02:15:04:05 -0700] "
)

const NEWLINE byte = 10

// FileOutput formats the output and then hands it off to the dataChan so the
// FileWriter can do its thing.
type FileOutput struct {
	dataRecycler DataRecycler
	outputBytes  []byte
	path         string
	format       string
	prefix_ts    bool
}

type FileOutputConfig struct {
	Path      string
	Format    string
	Prefix_ts bool
	Perm      os.FileMode
}

func (self *FileOutput) ConfigStruct() interface{} {
	return &FileOutputConfig{Format: "text", Perm: 0666}
}

// Initialize a WriteRunner, but only once.
func (self *FileOutput) Init(config interface{}) error {
	conf := config.(*FileOutputConfig)
	_, ok := FILEFORMATS[conf.Format]
	if !ok {
		return fmt.Errorf("Unsupported FileOutput format: %s", conf.Format)
	}
	// Using a map to guarantee there's only one WriteRunner is only safe b/c
	// the PipelinePacks (and therefore the FileOutputs) are initialized in
	// series. If this ever changes such that outputs might be created in
	// different threads then this will require a lock to make sure we don't
	// end up w/ two WriteRunners for the same file.
	self.dataRecycler, ok = FileDataRecyclers[conf.Path]
	if !ok {
		fileOutputWriter, err := NewFileOutputWriter(conf.Path, conf.Perm)
		if err != nil {
			return fmt.Errorf("Error creating FileOutputWriter: %s", err)
		}
		self.dataRecycler = NewDataRecycler(fileOutputWriter)
		FileDataRecyclers[conf.Path] = self.dataRecycler
	}
	self.path = conf.Path
	self.format = conf.Format
	self.prefix_ts = conf.Prefix_ts

	return nil
}

func (self *FileOutput) Deliver(pack *PipelinePack) {
	self.outputBytes = self.dataRecycler.RetrieveDataObject().([]byte)
	if self.prefix_ts {
		ts := time.Now().Format(TSFORMAT)
		self.outputBytes = append(self.outputBytes, ts...)
	}

	switch self.format {
	case "json":
		jsonMessage, err := json.Marshal(pack.Message)
		if err != nil {
			log.Printf("Error converting message to JSON for %s", self.path)
			return
		}
		self.outputBytes = append(self.outputBytes, jsonMessage...)
	case "text":
		self.outputBytes = append(self.outputBytes, pack.Message.Payload...)
	}
	self.outputBytes = append(self.outputBytes, NEWLINE)
	self.dataRecycler.SendOutputData(self.outputBytes)
}

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

// FileWriter implementation
var (
	FILEFORMATS = map[string]bool{
		"json": true,
		"text": true,
	}

	TSFORMAT = "[2006/Jan/02:15:04:05 -0700] "
)

const NEWLINE byte = 10

type FileWriter struct {
	path     string
	file     *os.File
	outBytes *[]byte
	ticker   *time.Ticker
}

type FileWriterConfig struct {
	Path      string
	Format    string
	Prefix_ts bool
	Perm      os.FileMode
}

func (self *FileWriter) ConfigStruct() interface{} {
	return &FileWriterConfig{Format: "text", Perm: 0666}
}

func (self *FileWriter) Init(config interface{}) (err error) {
	//path string, perm os.FileMode) (*FileOutputWriter,
	//error) {
	conf := config.(*FileWriterConfig)
	_, ok := FILEFORMATS[conf.Format]
	if !ok {
		return fmt.Errorf("Unsupported FileOutput format: %s", conf.Format)
	}
	self.path = conf.Path

	if self.file, err = os.OpenFile(path,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, perm); err == nil {
		go self.fileSyncer()
	}
	return err
}

func (self *FileWriter) fileSyncer() {
	self.ticker = time.NewTicker(time.Second)
	for _ = range self.ticker.C {
		self.file.Sync()
	}
}

func (self *FileWriter) MakeOutData() interface{} {
	b := make([]byte, 0, 2000)
	return &b
}

func (self *FileWriter) ZeroOutData(outData interface{}) {
	outBytes := outData.(*[]byte)
	*outBytes = (*outBytes)[:0]
}

func (self *FileWriter) PrepOutData(pack *PipelinePack, outData interface{}) {
	outBytes := outData.(*[]byte)
	if self.prefix_ts {
		ts := time.Now().Format(TSFORMAT)
		*outBytes = append(*outBytes, ts...)
	}

	switch self.format {
	case "json":
		jsonMessage, err := json.Marshal(pack.Message)
		if err != nil {
			log.Printf("Error converting message to JSON for %s", self.path)
			return
		}
		*outBytes = append(*outBytes, jsonMessage...)
	case "text":
		*outBytes = append(*outBytes, pack.Message.Payload...)
	}
	*outBytes = append(*outBytes, NEWLINE)
}

func (self *FileWriter) Write(outData interface{}) (err error) {
	self.outBytes = outData.(*[]byte)
	n, err := self.file.Write(*self.outBytes)
	if err != nil {
		err = fmt.Errorf("FileOutput error writing to %s: %s", self.path, err)
	} else if n != len(*self.outBytes) {
		err = fmt.Errorf("FileOutput truncated output for %s", self.path)
	}
	return
}

func (self *FileWriter) Event(eventType string) {
	if eventType == STOP {
		self.ticker.Stop()
		self.file.Close()
	}
}

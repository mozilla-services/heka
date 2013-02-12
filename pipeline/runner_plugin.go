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
	"errors"
	"github.com/rafrombrc/go-notify"
	"log"
	"runtime"
	"time"
)

type DataRecycler interface {
	// This must create exactly one instance of the `outData` data object type
	// expected by the `Write` method. Will be called multiple times to create
	// a pool of reusable objects.
	MakeOutData() (outData interface{})

	// Will be handed a used output object which should be reset to a zero
	// state for in preparation for reuse. This method will be in use by
	// multiple goroutines simultaneously, it should modify the passed
	// `outData` object **only**.
	ZeroOutData(outData interface{})

	// Extracts relevant information from the provided `PipelinePack`
	// (probably from the `Message` attribute) and uses it to populate the
	// provided output object. This method will be in use by multiple
	// goroutines simultaneously, it should modify the passed `outData` object
	// **only**. `timeout` will be nil unless the Runner plugin is being used
	// as an Input.
	PrepOutData(pack *PipelinePack, outData interface{}, timeout *time.Duration) error
}

// Interface for output objects that need to share a global resource (such as
// a file handle or network connection) to actually emit the output data.
type Writer interface {
	PluginGlobal
	DataRecycler

	// Setup method, called exactly once
	Init(config interface{}) error

	// Receives a populated output object, handles the actual work of writing
	// data out to an external destination.
	Write(outData interface{}) error
}

type BatchWriter interface {
	PluginGlobal
	DataRecycler

	// Setup method, called exactly once, returns a channel that ticks when the
	// Commit should be called
	Init(config interface{}) (<-chan time.Time, error)

	// Receives a populated output object, handles batching it as needed before
	// its to be committed
	Batch(outData interface{}) error

	// Called when a tick occurs to commit a batch
	Commit() error
}

// Plugin that drives a Writer or BatchWriter, instantiated many times
type Runner struct {
	Writer      Writer
	BatchWriter BatchWriter
	outData     interface{}
	global      *RunnerGlobal
}

// Global instance used by every runner
type RunnerGlobal struct {
	Recycler    DataRecycler
	Events      PluginGlobal
	dataChan    chan interface{}
	recycleChan chan interface{}
	ticker      <-chan time.Time
}

func (self *RunnerGlobal) Event(eventType string) {
	if self.Events != nil {
		self.Events.Event(eventType)
	}
}

func RunnerMaker(writer interface{}) interface{} {
	runner := new(Runner)
	if batch, ok := writer.(BatchWriter); ok {
		runner.BatchWriter = batch
	} else {
		runner.Writer = writer.(Writer)
	}
	return runner
}

func (self *Runner) InitOnce(config interface{}) (global PluginGlobal, err error) {
	conf := config.(*PluginConfig)
	g := new(RunnerGlobal)
	self.global = g
	var confLoaded interface{}

	// Determine how to initialize and if we hold a ticker
	if self.BatchWriter != nil {
		g.Recycler = self.BatchWriter.(DataRecycler)
		confLoaded, err = LoadConfigStruct(conf, self.BatchWriter)
		if g.ticker, err = self.BatchWriter.Init(confLoaded); err != nil {
			return g, errors.New("WriteRunner initialization error: " + err.Error())
		}
	} else {
		g.Recycler = self.Writer.(DataRecycler)
		confLoaded, err = LoadConfigStruct(conf, self.Writer)
		if err = self.Writer.Init(confLoaded); err != nil {
			return g, errors.New("WriteRunner initialization error: " + err.Error())
		}
	}
	if err != nil {
		return g, errors.New("WriteRunner config parsing error: " + err.Error())
	}

	g.dataChan = make(chan interface{}, 2*PoolSize)
	g.recycleChan = make(chan interface{}, 2*PoolSize)
	for i := 0; i < 2*PoolSize; i++ {
		g.recycleChan <- g.Recycler.MakeOutData()
	}

	if self.BatchWriter != nil {
		go self.batch_runner()
		return g, nil
	}
	go self.runner()
	return g, nil
}

func (self *Runner) Init(global PluginGlobal, config interface{}) error {
	self.global = global.(*RunnerGlobal)
	return nil
}

func (self *Runner) batch_runner() {
	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)
	var outData interface{}
	var err error
	for {
		// Yield before channel select can improve scheduler performance
		runtime.Gosched()
		select {
		case <-self.global.ticker:
			self.BatchWriter.Commit()
		case outData = <-self.global.dataChan:
			if err = self.BatchWriter.Batch(outData); err != nil {
				log.Println("OutputWriter error: ", err)
			}
			self.global.Recycler.ZeroOutData(outData)
			self.global.recycleChan <- outData
		case <-stopChan:
			return
		}
	}
}

func (self *Runner) runner() {
	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)
	var outData interface{}
	var err error
	for {
		// Yield before channel select can improve scheduler performance
		runtime.Gosched()
		select {
		case outData = <-self.global.dataChan:
			if err = self.Writer.Write(outData); err != nil {
				log.Println("OutputWriter error: ", err)
			}
			self.global.Recycler.ZeroOutData(outData)
			self.global.recycleChan <- outData
		case <-stopChan:
			return
		}
	}
}

func (self *Runner) Deliver(pack *PipelinePack) {
	self.outData = <-self.global.recycleChan
	err := self.global.Recycler.PrepOutData(pack, self.outData, nil)
	if err != nil {
		log.Printf("PrepOutData error: %s", err.Error())
		self.global.Recycler.ZeroOutData(self.outData)
		self.global.recycleChan <- self.outData
		return
	}
	self.global.dataChan <- self.outData
}

func (self *Runner) FilterMsg(pipelinePack *PipelinePack) {
	self.outData = <-self.global.recycleChan
	err := self.global.Recycler.PrepOutData(pipelinePack, self.outData, nil)
	if err != nil {
		log.Printf("PrepOutData error: %s", err.Error())
		self.global.Recycler.ZeroOutData(self.outData)
		self.global.recycleChan <- self.outData
		return
	}
	self.global.dataChan <- self.outData
}

func (self *Runner) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) (err error) {
	self.outData = <-self.global.recycleChan
	err = self.global.Recycler.PrepOutData(pipelinePack, self.outData, timeout)
	if err != nil {
		self.global.Recycler.ZeroOutData(self.outData)
		self.global.recycleChan <- self.outData
		return
	}
	self.global.dataChan <- self.outData
	return
}

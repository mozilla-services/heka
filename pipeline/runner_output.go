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
	MakeOutData() interface{}

	// Will be handed a used output object which should be reset to a zero
	// state for in preparation for reuse. This method will be in use by
	// multiple goroutines simultaneously, it should modify the passed
	// `outData` object **only**.
	ZeroOutData(outData interface{})

	// Extracts relevant information from the provided `PipelinePack`
	// (probably from the `Message` attribute) and uses it to populate the
	// provided output object. This method will be in use by multiple
	// goroutines simultaneously, it should modify the passed `outData` object
	// **only**.
	PrepOutData(pack *PipelinePack, outData interface{})
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
	Init(config interface{}) (chan<- time.Time, error)

	// Receives a populated output object, handles batching it as needed before
	// its to be committed
	Batch(outData interface{}) error

	// Called when a tick occurs to commit a batch
	Commit() error
}

// Plugin that drives a Writer or BatchWriter
type Runner struct {
	Writer      Writer
	BatchWriter BatchWriter
	Recycler    DataRecycler
	dataChan    chan interface{}
	recycleChan chan interface{}
	outData     interface{}
	ticker      chan time.Time
}

func RunnerMaker(writer interface{}) func() interface{} {
	makeRunner := func() interface{} {
		runner := new(Runner)
		if batch, ok := writer.(BatchWriter); ok {
			runner.BatchWriter = batch
		} else {
			runner.Writer = writer.(Writer)
		}
		runner.Recycler = writer.(DataRecycler)
	}
	return func() interface{} { return makeRunner() }
}

func (self *Runner) InitOnce(config interface{}) (global PluginGlobal, err error) {
	conf := config.(*PluginConfig)
	confLoaded, err := LoadConfigStruct(conf, self.Writer)
	if err != nil {
		return self.Writer, errors.New("WriteRunner config parsing error: " + err.Error())
	}

	// Determine how to initialize and if we hold a ticker
	if self.BatchWriter != nil {
		if self.ticker, err = self.BatchWriter.Init(confLoaded); err != nil {
			return self.BatchWriter, errors.New("WriteRunner initialization error: " + err.Error())
		} else {
			go self.batch_runner()
		}
	} else {
		if err = self.Writer.Init(confLoaded); err != nil {
			return self.Writer, errors.New("WriteRunner initialization error: " + err.Error())
		} else {
			go self.runner()
		}
	}

	self.dataChan = make(chan interface{}, 2*PoolSize)
	self.recycleChan = make(chan interface{}, 2*PoolSize)
	for i := 0; i < 2*PoolSize; i++ {
		self.recycleChan <- self.Recycler.MakeOutData()
	}

	if self.BatchWriter != nil {
		return self.BatchWriter, nil
	} else {
		return self.Writer, nil
	}
}

func (self *Runner) Init(global PluginGlobal, config interface{}) error {
	return nil
}

func (self *Runner) batch_runner() {
	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)
	var outData interface{}
	var tick time.Time
	var err error
	for {
		// Yield before channel select can improve scheduler performance
		runtime.Gosched()
		select {
		case tick = <-self.ticker:
			self.BatchWriter.Commit()
		case outData = <-self.dataChan:
			if err = self.BatchWriter.Batch(outData); err != nil {
				log.Println("OutputWriter error: ", err)
			}
			self.Recycler.ZeroOutData(outData)
			self.recycleChan <- outData
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
		case outData = <-self.dataChan:
			if err = self.Writer.Write(outData); err != nil {
				log.Println("OutputWriter error: ", err)
			}
			self.Recycler.ZeroOutData(outData)
			self.recycleChan <- outData
		case <-stopChan:
			return
		}
	}
}

func (self *Runner) Deliver(pack *PipelinePack) {
	self.outData = <-self.recycleChan
	self.Recycler.PrepOutData(pack, self.outData)
	self.dataChan <- self.outData
}

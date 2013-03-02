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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	. "github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/go-notify"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	// Control channel event types used by go-notify
	RELOAD = "reload"
	STOP   = "stop"

	// buffer size for plugin channels
	PIPECHAN_BUFSIZE = 500
)

var PoolSize int

type Plugin interface {
	Init(config interface{}) error
}

type PluginGlobal interface {
	// Called when an event occurs, either RELOAD or STOP
	Event(eventType string)
}

type PluginWithGlobal interface {
	Init(global PluginGlobal, config interface{}) error
	InitOnce(config interface{}) (global PluginGlobal, err error)
}

type PluginRunnerBase interface {
	InChan() chan *PipelinePack
	Name() string
}

type Startable interface {
	Start(wg *sync.WaitGroup)
}

// Base struct for the specialized PluginRunners
type pluginRunnerBase struct {
	inChan chan *PipelinePack
	name   string
}

func (self *pluginRunnerBase) InChan() chan *PipelinePack {
	return self.inChan
}

func (self *pluginRunnerBase) Name() string {
	return self.name
}

type PipelinePack struct {
	MsgBytes   []byte
	Message    *Message
	Config     *PipelineConfig
	Decoder    string
	Decoders   map[string]Decoder
	Decoded    bool
	ChainCount int
	RefCount   int32
}

func NewPipelinePack(config *PipelineConfig) *PipelinePack {
	msgBytes := make([]byte, MAX_MESSAGE_SIZE)
	message := &Message{}
	decoders := make(map[string]Decoder)

	pack := &PipelinePack{
		MsgBytes: msgBytes,
		Message:  message,
		Config:   config,
		Decoders: decoders,
		Decoded:  false,
		RefCount: int32(1),
	}
	pack.InitDecoders(config)
	return pack
}

func (self *PipelinePack) InitDecoders(config *PipelineConfig) {
	for name, wrapper := range config.Decoders {
		self.Decoders[name] = wrapper.Create().(Decoder)
	}
}

func (self *PipelinePack) Zero() {
	self.MsgBytes = self.MsgBytes[:cap(self.MsgBytes)]
	self.Decoded = false
	self.RefCount = 1

	// TODO: Possibly zero the message instead depending on benchmark
	// results of re-allocating a new message
	self.Message = new(Message)
}

func (self *PipelinePack) Recycle() {
	cnt := atomic.AddInt32(&self.RefCount, -1)
	if cnt == 0 {
		self.Zero()
		self.Config.RecycleChan <- self
	}
}

func BroadcastEvent(config *PipelineConfig, eventType string) {
	err := notify.Post(eventType, nil)
	if err != nil {
		log.Printf("Error sending %s event:", err.Error())
	}

	var wrapper *PluginWrapper
	for _, wrapper = range config.Filters {
		if wrapper.global != nil {
			wrapper.global.Event(eventType)
		}
	}
	for _, wrapper = range config.Outputs {
		if wrapper.global != nil {
			wrapper.global.Event(eventType)
		}
	}
}

func Run(config *PipelineConfig) {
	log.Println("Starting hekad...")

	var wg sync.WaitGroup
	var outRunner *outputRunner

	for name, wrapper := range config.Outputs {
		output := wrapper.Create().(Output)
		// Band-aid to check if output needs to be started. This will go away
		// when we finish the pipeline redesign.
		var oi interface{} = output
		if s, ok := oi.(Startable); ok {
			s.Start(&wg)
		}

		outRunner = newOutputRunner(name, output)
		config.OutputRunners[name] = outRunner
		outRunner.Start(&wg)
		wg.Add(1)
		log.Printf("Output started: %s\n", name)
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < config.PoolSize; i++ {
		config.RecycleChan <- NewPipelinePack(config)
	}

	for name, wrapper := range config.Inputs {
		inputPlug, err := wrapper.CreateWithError()
		if err != nil {
			log.Fatalf("Failure to load plugin: %s", name)
		}
		input := inputPlug.(Input)
		input.SetName(name)
		if input.Start(config, &wg) != nil {
			log.Printf("'%s' input failed to start: %s", name, err)
			continue
		}
		wg.Add(1)
		log.Printf("Input started: %s\n", name)
	}

	// wait for sigint
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP)

sigListener:
	for {
		select {
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				log.Println("Reload initiated.")
				BroadcastEvent(config, RELOAD)
			case syscall.SIGINT:
				log.Println("Shutdown initiated.")
				BroadcastEvent(config, STOP)
				break sigListener
			}
		}
	}

	wg.Wait()
	log.Println("Shutdown complete.")
}

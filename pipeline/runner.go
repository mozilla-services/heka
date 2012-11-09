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
	. "heka/message"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Control channel event types used by go-notify
const (
	RELOAD = "reload"
	STOP   = "stop"
)

type Plugin interface {
	Init(config interface{}) error
}

type PipelinePack struct {
	MsgBytes    []byte
	Message     *Message
	Config      *PipelineConfig
	Decoder     string
	Decoders    map[string]Decoder
	Filters     map[string]Filter
	Outputs     map[string]Output
	Decoded     bool
	FilterChain string
	ChainCount  int
	OutputNames map[string]bool
}

func NewPipelinePack(config *PipelineConfig) *PipelinePack {
	msgBytes := make([]byte, 65536)
	message := Message{}
	outputnames := make(map[string]bool)
	filters := make(map[string]Filter)
	decoders := make(map[string]Decoder)
	outputs := make(map[string]Output)

	pack := &PipelinePack{
		MsgBytes:    msgBytes,
		Message:     &message,
		Config:      config,
		Decoder:     config.DefaultDecoder,
		Decoders:    decoders,
		Decoded:     false,
		Filters:     filters,
		FilterChain: config.DefaultFilterChain,
		Outputs:     outputs,
		OutputNames: outputnames,
	}
	pack.InitDecoders(config)
	pack.InitFilters(config)
	pack.InitOutputs(config)
	return pack
}

func (self *PipelinePack) InitDecoders(config *PipelineConfig) {
	for name, plugin := range config.DecoderCreator() {
		self.Decoders[name] = plugin
	}
}

func (self *PipelinePack) InitFilters(config *PipelineConfig) {
	for name, plugin := range config.FilterCreator() {
		self.Filters[name] = plugin
	}
}

func (self *PipelinePack) InitOutputs(config *PipelineConfig) {
	for name, plugin := range config.OutputCreator() {
		self.Outputs[name] = plugin
	}
}

func filterProcessor(pipelinePack *PipelinePack) {
	pipelinePack.OutputNames = map[string]bool{}
	config := pipelinePack.Config
	filterChainName, ok := config.Lookup.LocateChain(pipelinePack.Message)
	if ok {
		pipelinePack.FilterChain = filterChainName
	} else {
		filterChainName = pipelinePack.FilterChain
	}
	filterChain, ok := config.FilterChains[filterChainName]
	if !ok {
		log.Printf("Filter chain doesn't exist: %s", filterChainName)
		return
	}
	for _, outputName := range filterChain.Outputs {
		pipelinePack.OutputNames[outputName] = true
	}
	for _, filterName := range filterChain.Filters {
		filter := pipelinePack.Filters[filterName]
		filter.FilterMsg(pipelinePack)
		if pipelinePack.Message == nil {
			return
		}
	}
}

func NewPipelineConfig(poolSize int) (config *PipelineConfig) {
	config = new(PipelineConfig)
	config.PoolSize = poolSize
	config.Inputs = make(map[string]Input)
	config.FilterChains = make(map[string]FilterChain)
	config.DefaultFilterChain = "default"
	config.Lookup = new(MessageLookup)
	config.Lookup.MessageType = make(map[string][]string)
	return config
}

func (self *PipelineConfig) Run() {
	log.Println("Starting hekad...")

	// Used for recycling PipelinePack objects
	recycleChan := make(chan *PipelinePack, self.PoolSize+1)

	// Main pipeline function, inputs spawn a goroutine of this for every
	// message
	pipeline := func(pack *PipelinePack) {

		// When finished, reset and recycle the allocated PipelinePack
		defer func() {
			pack.MsgBytes = pack.MsgBytes[:cap(pack.MsgBytes)]
			pack.Decoder = self.DefaultDecoder
			pack.Decoded = false
			pack.FilterChain = self.DefaultFilterChain
			outputs := make(map[string]bool)
			pack.OutputNames = outputs
			recycleChan <- pack
		}()

		// Decode message if necessary
		if !pack.Decoded {
			decoderName := pack.Decoder
			decoder, ok := pack.Decoders[decoderName]
			if !ok {
				log.Printf("Decoder doesn't exist: %s\n", decoderName)
				return
			}
			err := decoder.Decode(pack)
			if err != nil {
				log.Fatalf("Error decoding message (%s): %s", decoderName,
					err.Error())
				return
			}
		}

		// Run message through the appropriate filters
		filterProcessor(pack)
		if pack.Message == nil {
			return
		}

		// Deliver message to appropriate outputs
		for outputName, use := range pack.OutputNames {
			if !use {
				continue
			}
			output, ok := pack.Outputs[outputName]
			if !ok {
				log.Printf("Output doesn't exist: %s\n", outputName)
			}
			output.Deliver(pack)
		}
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < self.PoolSize; i++ {
		recycleChan <- NewPipelinePack(self)
	}

	var wg sync.WaitGroup
	var runner InputRunner
	timeout := time.Duration(time.Second / 2)
	inputRunners := make(map[string]*InputRunner)

	for name, input := range self.Inputs {
		runner = InputRunner{input, &timeout, false}
		inputRunners[name] = &runner
		runner.Start(pipeline, recycleChan, &wg)
		wg.Add(1)
		log.Printf("Input started: %s\n", name)
	}

	// wait for sigint
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT)
	for {
		sigint := <-sigChan
		if sigint != nil {
			break
		}
	}

	for name, runner := range inputRunners {
		runner.Stop()
		log.Printf("Stopping input: %s\n", name)
	}
	wg.Wait()
	log.Println("Shutdown complete.")
}

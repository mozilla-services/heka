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

type Plugin interface {
	Init(config interface{}) error
}

type PluginJsonConfig interface {
	Plugin
	JsonConfig() interface{}
}

type PipelinePack struct {
	MsgBytes    []byte
	Message     *Message
	Config      *GraterConfig
	Decoder     string
	Decoded     bool
	FilterChain string
	ChainCount  int
	Outputs     map[string]bool
}

func NewPipelinePack(config *GraterConfig) *PipelinePack {
	msgBytes := make([]byte, 65536)
	message := Message{}
	outputs := make(map[string]bool)
	for _, outputName := range config.DefaultOutputs {
		outputs[outputName] = true
	}
	pipelinePack := PipelinePack{
		MsgBytes:    msgBytes,
		Message:     &message,
		Config:      config,
		Decoder:     config.DefaultDecoder,
		Decoded:     false,
		FilterChain: config.DefaultFilterChain,
		Outputs:     outputs,
	}
	return &pipelinePack
}

func filterProcessor(pipelinePack *PipelinePack) {
	pipelinePack.Outputs = map[string]bool{}
	config := pipelinePack.Config
	filterChainName := pipelinePack.FilterChain
	filterChain, ok := config.FilterChains[filterChainName]
	if !ok {
		log.Printf("Filter chain doesn't exist: %s", filterChainName)
		return
	}
	for _, outputName := range filterChain.Outputs {
		pipelinePack.Outputs[outputName] = true
	}
	for _, filterName := range filterChain.Filters {
		filter := config.Filters[filterName]
		filter.FilterMsg(pipelinePack)
		if pipelinePack.Message == nil {
			return
		}
	}
}

func Run(config *GraterConfig) {
	log.Println("Starting hekagrater...")

	// Used for recycling PipelinePack objects
	recycleChan := make(chan *PipelinePack, config.PoolSize+1)

	// Main pipeline function, inputs spawn a goroutine of this for every
	// message
	pipeline := func(pipelinePack *PipelinePack) {

		// When finished, reset and recycle the allocated PipelinePack
		defer func() {
			msgBytes := pipelinePack.MsgBytes
			msgBytes = msgBytes[:cap(msgBytes)]
			pipelinePack.Decoder = config.DefaultDecoder
			pipelinePack.Decoded = false
			pipelinePack.FilterChain = config.DefaultFilterChain
			outputs := make(map[string]bool)
			for _, outputName := range config.DefaultOutputs {
				outputs[outputName] = true
			}
			pipelinePack.Outputs = outputs
			recycleChan <- pipelinePack
		}()

		// Decode messgae if necessary
		if !pipelinePack.Decoded {
			decoderName := pipelinePack.Decoder
			decoder, ok := config.Decoders[decoderName]
			if !ok {
				log.Printf("Decoder doesn't exist: %s\n", decoderName)
				return
			}
			err := decoder.Decode(pipelinePack)
			if err != nil {
				log.Printf("Error decoding message (%s decoder): %s",
					decoderName, err.Error())
				return
			}
		}

		// Run message through the appropriate filters
		filterProcessor(pipelinePack)
		if pipelinePack.Message == nil {
			return
		}

		// Deliver message to appropriate outputs
		for outputName, use := range pipelinePack.Outputs {
			if !use {
				continue
			}
			output, ok := config.Outputs[outputName]
			if !ok {
				log.Printf("Output doesn't exist: %s\n", outputName)
			}
			output.Deliver(pipelinePack)
		}
	}

	// Initialize all of the PipelinePacks that we'll need
	for i := 0; i < config.PoolSize; i++ {
		recycleChan <- NewPipelinePack(config)
	}

	var wg sync.WaitGroup
	var runner InputRunner
	timeout := time.Duration(time.Second / 2)
	inputRunners := make(map[string]*InputRunner)

	for name, input := range config.Inputs {
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

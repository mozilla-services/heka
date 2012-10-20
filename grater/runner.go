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
package hekagrater

import (
	"heka/message"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Message hekamessage.Message

type GraterConfig struct {
	Inputs map[string]Input
	Decoders map[string]Decoder
	DefaultDecoder string
	Filters []Filter
	Outputs map[string]Output
	DefaultOutputs []string
	PoolSize int
}

type PipelinePack struct {
	MsgBytes *[]byte
}

func decodeDelegator(msgBytes *[]byte, decoders *map[string]Decoder,
                     defaultDecoder *string) Decoder {
	var decoderName *string
	if (*msgBytes)[0] == 0 {
		log.Println("TODO: Wire protocol not yet implemented")
		return nil
	} else {
		decoderName = defaultDecoder
	}
	decoder, ok := (*decoders)[*decoderName]
	if !ok {
		log.Printf("Decoder doesn't exist: %s\n", *decoderName)
		return nil
	}
	return decoder
}

func filterProcessor(msg *Message, config *GraterConfig) (*Message,
	                                                  *map[string]bool) {
	outputs := make(map[string]bool)
	for _, outputName := range config.DefaultOutputs {
		outputs[outputName] = true
	}
	for _, filter := range config.Filters {
		filter.FilterMsg(msg, &outputs)
		if msg == nil {
			return nil, nil
		}
	}
	return msg, &outputs
}

func Run(config *GraterConfig) {
	log.Println("Starting hekagrater...")

	recycleChan := make(chan *PipelinePack, config.PoolSize+1)
	pipeline := func(pipelinePack *PipelinePack) {
		defer func() {
			// recycle the allocated PipelinePack
			recycleChan <- pipelinePack
		}()

		msgBytes := pipelinePack.MsgBytes
		decoder := decodeDelegator(msgBytes, &config.Decoders,
			                   &config.DefaultDecoder)
		if decoder == nil {
			return
		}
		msg := decoder.Decode(pipelinePack)

		msg, outputNames := filterProcessor(msg, config)
		if msg == nil {
			return
		}

		for outputName, _ := range *outputNames {
			output, ok := config.Outputs[outputName]
			if !ok {
				log.Printf("Output doesn't exist: %s\n", outputName)
			}
			output.Deliver(msg)
		}
	}

	for i := 0; i < config.PoolSize; i++ {
		msgBytes := make([]byte, 65536)
		pipelinePack := PipelinePack{
			MsgBytes: &msgBytes,
		}
		recycleChan <- &pipelinePack
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

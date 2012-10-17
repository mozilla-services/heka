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
	"syscall"
)

type Message hekamessage.Message

type GraterConfig struct {
	Inputs map[string]Input
	Decoders map[string]Decoder
	DefaultDecoder string
	Filters []Filter
	Outputs map[string]Output
	DefaultOutputs []string
	PipelinePoolSize int
}

type PipelinePack struct {
	MsgBytes *[]byte
	PluginData map[string]map[string]interface{}
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

	recycleChan := make(chan *PipelinePack, config.PipelinePoolSize)
	pipeline := func(pipelinePack *PipelinePack) {
		defer func() {
			// recycle the allocated PipelinePack
			msgBytes := pipelinePack.MsgBytes
			msgBytesFull := (*msgBytes)[:cap(*msgBytes)]
			pipelinePack.MsgBytes = &msgBytesFull
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

	for i := 0; i < config.PipelinePoolSize; i++ {
		msgBytes := make([]byte, 65536)
		pluginData := make(map[string]map[string]interface{})
		pipelinePack := PipelinePack{&msgBytes, pluginData}
		recycleChan <- &pipelinePack
	}

	for name, input := range config.Inputs {
		input.Start(pipeline, recycleChan)
		log.Printf("Input started: %s\n", name)
	}

	// wait for sigint
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT)
	for {
		sigint := <-sigChan
		if sigint != nil {
			log.Println("Clean shutdown.")
			break
		}
	}
}

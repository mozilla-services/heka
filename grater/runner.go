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

const (
	timeFormat = "2006-01-02T15:04:05.000000"
	timeFormatFullSecond = "2006-01-02T15:04:05"
)

type Message hekamessage.Message

type GraterConfig struct {
	Inputs map[string]Input
	Decoders map[string]Decoder
	DefaultDecoder string
	Filters []Filter
	Outputs map[string]Output
	DefaultOutputs []string
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

	pipeline := func(msgBytes *[]byte, recycleChan chan<- *[]byte) {
		defer func() {
			// recycle message buffer if we can, or skip it if the
			// recycle channel buffer is already full
			select {
			case recycleChan <- msgBytes:
			default:
			}
		}()

		decoder := decodeDelegator(msgBytes, &config.Decoders,
			                   &config.DefaultDecoder)
		if decoder == nil {
			return
		}
		msg := decoder.Decode(msgBytes)

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

	for name, input := range config.Inputs {
		input.Start(pipeline)
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

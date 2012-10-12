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
	"log"
	"github.com/bitly/go-simplejson"
	"sync"
	"time"
)

const (
	timeFormat = "2006-01-02T15:04:05.000000"
)

type Message struct {
	Type string
	Timestamp time.Time
	Logger string
	Severity int
	Payload string
	Fields *simplejson.Json
	Env_version string
	Pid int
	Hostname string
}

type GraterConfig struct {
	Inputs map[string]Input
	Decoders map[string]Decoder
	DefaultDecoder string
	Filters []Filter
	Outputs map[string]Output
	DefaultOutputs []string
}

func decodeDelegator(inChan <-chan *[]byte, decoderChans *map[string]chan *[]byte,
                     defaultDecoder string) {
	var decoder string
	var decoderChan chan *[]byte
	var ok bool
	for {
		msgBytes := <-inChan
		if (*msgBytes)[0] == 0 {
			log.Println("TODO: Wire protocol not yet implemented")
			decoder = defaultDecoder
		} else {
			decoder = defaultDecoder
		}
		decoderChan, ok = (*decoderChans)[decoder]
		if !ok {
			log.Printf("Decoder doesn't exist: %s\n", decoder)
		}
		decoderChan <- msgBytes
	}
}

func filterProcessor(inChan <-chan *Message, outputChans *map[string]chan *Message,
                     config *GraterConfig) {
	for {
		msg := <-inChan
		outputs := make(map[string]bool)
		for _, outputName := range config.DefaultOutputs {
			outputs[outputName] = true
		}
		for _, filter := range config.Filters {
			filter.FilterMsg(msg, &outputs)
			if msg == nil {
				break
			}
		}
		if msg == nil {
			continue
		}
		for outputName, _ := range outputs {
			outputChan, ok := (*outputChans)[outputName]
			if !ok {
				log.Printf("Output doesn't exist: %s", outputName)
				continue
			}
			outputChan <- msg
		}
	}
}

func Run(config *GraterConfig) {
	log.Println("Starting hekagrater...")

	receivedChan := make(chan *[]byte)
	decodedChan := make(chan *Message)
	decoderChans := make(map[string]chan *[]byte)
	outputChans := make(map[string]chan *Message)

	for name, input := range config.Inputs {
		input.Start(receivedChan)
		log.Printf("Input started: %s\n", name)
	}

	for name, decoder := range config.Decoders {
		runner := DecoderRunner{decoder}
		decoderChan := runner.Start(decodedChan)
		log.Printf("Decoder started: %s\n", name)
		decoderChans[name] = decoderChan
	}
	go decodeDelegator(receivedChan, &decoderChans, config.DefaultDecoder)
	log.Println("Decode delegator started")

	for name, output := range config.Outputs {
		runner := OutputRunner{output}
		outputChan := runner.Start()
		log.Printf("Output started: %s\n", name)
		outputChans[name] = outputChan
	}

	go filterProcessor(decodedChan, &outputChans, config)
	log.Println("Filter processor started")

	// wait forever
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}

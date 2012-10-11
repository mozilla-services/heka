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
	Outputs map[string]Output
	DefaultDecoder string
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

func outputter(decodedChan chan *Message, outputChan chan *Message) {
	for {
		msg := <-decodedChan
		outputChan <- msg
	}
}

func Run(config *GraterConfig) {
	log.Println("Starting hekagrater...")

	receivedChan := make(chan *[]byte)
	for name, input := range config.Inputs {
		go input.Start(receivedChan)
		log.Printf("Input started: %s\n", name)
	}

	decodedChan := make(chan *Message)
	decoderChans := make(map[string]chan *[]byte)
	for name, decoder := range config.Decoders {
		runner := DecoderRunner{decoder}
		decoderChan := runner.Start(decodedChan)
		log.Printf("Decoder started: %s\n", name)
		decoderChans[name] = decoderChan
	}
	go decodeDelegator(receivedChan, &decoderChans, config.DefaultDecoder)
	log.Println("Decode delegator started")

	for _, output := range config.Outputs {
		runner := OutputRunner{output}
		outputChan := runner.Start()
		go outputter(decodedChan, outputChan)
	}
	log.Println("Output started")

	// wait forever
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}

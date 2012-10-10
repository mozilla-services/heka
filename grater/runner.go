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
	Inputs []Input
	Outputs []Output
}

func decode(msgBytes []byte) (Message, error) {
	msgJson, err := simplejson.NewJson(msgBytes)
	var msg Message
	msg.Type = msgJson.Get("type").MustString()
	timeStr := msgJson.Get("timestamp").MustString()
	msg.Timestamp, _ = time.Parse(timeFormat, timeStr)
	msg.Logger = msgJson.Get("logger").MustString()
	msg.Severity = msgJson.Get("severity").MustInt()
	msg.Payload, _ = msgJson.Get("payload").String()
	msg.Fields = msgJson.Get("fields")
	msg.Env_version = msgJson.Get("env_version").MustString()
	msg.Pid, _ = msgJson.Get("metlog_pid").Int()
	msg.Hostname, _ = msgJson.Get("metlog_hostname").String()
	return msg, err
}

func decoder(receivedChan chan []byte, decodedChan chan Message) {
	for {
		msgBytes := <-receivedChan
		msg, _ := decode(msgBytes)
		decodedChan <- msg
	}
}

func outputter(decodedChan chan Message, outputChan chan *Message) {
	for {
		msg := <-decodedChan
		outputChan <- &msg
	}
}

func Run(config *GraterConfig) {
	log.Println("hekagrater.Run()")

	receivedChan := make(chan []byte)
	for _, input := range config.Inputs {
		go input.Start(receivedChan)
	}
	log.Println("Inputs started")

	decodedChan := make(chan Message)
	go decoder(receivedChan, decodedChan)
	log.Println("Decoder started")

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

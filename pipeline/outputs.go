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
	"github.com/peterbourgon/g2s"
	"log"
	"runtime"
	"strconv"
	"time"
)

type StatsdClient interface {
	// Interface that all statsd clients must implement
	IncrementSampledCounter(bucket string, n int, srate float32)
	SendSampledTiming(bucket string, ms int, srate float32)
}

type Output interface {
	Plugin
	Deliver(pipelinePack *PipelinePack)
}

type LogOutput struct {
}

func (self *LogOutput) Init(config interface{}) error {
	return nil
}

func (self *LogOutput) Deliver(pipelinePack *PipelinePack) {
	log.Printf("%+v\n", *(pipelinePack.Message))
}

type CounterOutput struct {
	count uint
}

func (self *CounterOutput) Init(config interface{}) error {
	ticker := time.NewTicker(time.Duration(time.Second))
	go self.timerLoop(ticker)
	return nil
}

func (self *CounterOutput) Deliver(pipelinePack *PipelinePack) {
	self.count++
	runtime.Gosched()
}

func (self *CounterOutput) timerLoop(ticker *time.Ticker) {
	lastTime := time.Now()
	lastCount := self.count
	zeroes := int8(0)
	var (
		msgsSent, newCount uint
		elapsedTime        time.Duration
		now                time.Time
		rate               float64
	)
	for {
		_ = <-ticker.C
		newCount = self.count
		now = time.Now()
		msgsSent = newCount - lastCount
		lastCount = newCount
		elapsedTime = now.Sub(lastTime)
		lastTime = now
		rate = float64(msgsSent) / elapsedTime.Seconds()
		if msgsSent == 0 {
			if newCount == 0 || zeroes == 3 {
				continue
			}
			zeroes++
		} else {
			zeroes = 0
		}
		log.Printf("Got %d messages. %0.2f msg/sec\n", newCount, rate)
	}
}

type StatsdOutput struct {
	statsdClient StatsdClient
}

func NewStatsdClient(url string) StatsdClient {
	sd, sd_err := g2s.NewStatsd(url)
	if sd_err != nil {
		log.Printf("Error!! No statsd client was created! %v", sd_err)
	} else {
		log.Printf("g2s statsd client created")
	}
	return sd
}

func (self *StatsdOutput) Init(config interface{}) error {
	self.statsdClient = NewStatsdClient("localhost:5565")
	return nil
}

func (self *StatsdOutput) runLoop() {
	for {
		runtime.Gosched()
	}
}

func (self *StatsdOutput) Deliver(pipelinePack *PipelinePack) {

	msg := pipelinePack.Message

	// TODO: we need the ns for the full key
	// ns := msg.Logger

	key := msg.Fields["name"].(string)

	tmp_value, value_err := strconv.ParseInt(msg.Payload, 10, 32)
	if value_err != nil {
		// Do something on error
	}
	// Downcast this
	value := int(tmp_value)

	rate := float32(msg.Fields["rate"].(float64))

	switch msg.Type {
	case "counter":
		// TODO: Handle the ns
		self.statsdClient.IncrementSampledCounter("foo", 2, 1)
	case "timer":
		// TODO: handle the ns
		//self.statsdClient.SendSampledTiming(key, value, rate)
	default:
		// TODO: need a better warning logger when things go wrong
		log.Printf("Warning: Unexpected event passed into StatsdOutput.\nEvent => %+v\n", *(pipelinePack.Message))
	}
	log.Printf("key: %v value: %v rate: %v", key, value, rate)
	log.Printf("==================")
	runtime.Gosched()
}

func NewStatsdOutput(statsdClient StatsdClient) *StatsdOutput {
	self := StatsdOutput{}
	go self.runLoop()
	return &self
}

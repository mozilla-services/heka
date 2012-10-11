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
	"time"
)

type Output interface {
	deliver(msg *Message)
}

type OutputRunner struct {
	output Output
}

func (self *OutputRunner) Start() chan *Message {
	inChan := make(chan *Message)
	go self.run(inChan)
	return inChan
}

func (self *OutputRunner) run(inChan <-chan *Message) {
	for {
		msg := <- inChan
		go self.output.deliver(msg)
	}
}

type LogOutput struct {
}

func (self *LogOutput) deliver(msg *Message) {
	log.Printf("%+v\n", msg)
}

type counterOutput struct {
	count uint
}

func NewCounterOutput () *counterOutput {
	self := counterOutput{0}
	ticker := time.NewTicker(time.Duration(time.Second))
	go self.timerLoop(ticker)
	return &self
}

func (self *counterOutput) deliver(msg *Message) {
	self.count++
}

func (self *counterOutput) timerLoop(ticker *time.Ticker) {
	lastTime := time.Now()
	lastCount := self.count
	zeroes := int8(0)
	var (
		msgsSent, newCount uint
		elapsedTime time.Duration
		now time.Time
		rate float64
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
			if zeroes == 3 {
				continue
			}
			zeroes++
		} else {
			zeroes = 0
		}
		log.Printf("Got %d messages. %0.2f msg/sec\n", newCount, rate)
	}
}

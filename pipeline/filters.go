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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

type FilterRunner interface {
	PluginRunner
	Filter() Filter
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	Ticker() (ticker <-chan time.Time)
}

type Filter interface {
	Start(r FilterRunner, h PluginHelper, wg *sync.WaitGroup) (err error)
}

type CounterFilter struct {
	lastTime  time.Time
	lastCount uint
	count     uint
	zeroes    int8
	rate      float64
	rates     []float64
	intervals uint
}

func (this *CounterFilter) Init(config interface{}) error {
	this.lastTime = time.Now()
	return nil
}

func (this *CounterFilter) Start(fr FilterRunner, h PluginHelper,
	wg *sync.WaitGroup) (err error) {

	inChan := fr.InChan()
	ticker := fr.Ticker()

	go func() {
		ok := true
		for ok {
			select {
			case _, ok = <-inChan:
				if !ok {
					break
				}
				this.count++
			case <-ticker:
				this.tally()
			}
		}

		log.Printf("CounterFilter '%s' stopped.", fr.Name())
		wg.Done()
	}()
	return
}

func (this *CounterFilter) tally() {
	this.intervals++
	now := time.Now()
	msgsSent := this.count - this.lastCount
	elapsedTime := now.Sub(this.lastTime)
	this.lastCount = this.count
	this.lastTime = now
	this.rate = float64(msgsSent) / elapsedTime.Seconds()
	if msgsSent == 0 {
		if msgsSent == 0 || this.zeroes == 3 {
			return
		}
		this.zeroes++
	} else {
		this.zeroes = 0
	}

	outMsg := MessageGenerator.Retrieve()
	outMsg.Message.SetType("heka.counter_output")
	outMsg.Message.SetPayload(fmt.Sprintf("Got %d messages. %0.2f msg/sec",
		this.count, this.rate))
	MessageGenerator.Inject(outMsg)

	this.rates = append(this.rates, this.rate)
	if this.intervals >= 10 {
		this.intervals = 0
		amount := len(this.rates)
		if amount < 1 {
			return
		}
		sort.Float64s(this.rates)
		min := this.rates[0]
		max := this.rates[amount-1]
		mean := min
		sum := float64(0)
		for _, val := range this.rates {
			sum += val
		}
		mean = sum / float64(amount)
		outMsg = MessageGenerator.Retrieve()
		outMsg.Message.SetType("heka.counter_output")
		outMsg.Message.SetPayload(
			fmt.Sprintf("AGG Sum. Min: %0.2f    Max: %0.2f    Mean: %0.2f",
				min, max, mean))
		MessageGenerator.Inject(outMsg)
		this.rates = this.rates[:0]
	}
}

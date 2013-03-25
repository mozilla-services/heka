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
	"sort"
	"sync"
	"time"
)

type FilterRunner interface {
	PluginRunner
	InChan() chan *PipelineCapture
	Filter() Filter
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	Ticker() (ticker <-chan time.Time)
	Deliver(pack *PipelinePack)
}

type Filter interface {
	Run(r FilterRunner, h PluginHelper) (err error)
}

type CounterFilter struct {
	lastTime  time.Time
	lastCount uint
	count     uint
	rate      float64
	rates     []float64
}

func (this *CounterFilter) Init(config interface{}) error {
	return nil
}

func (this *CounterFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()
	ticker := fr.Ticker()
	this.lastTime = time.Now()

	var (
		ok  = true
		plc *PipelineCapture
	)
	for ok {
		select {
		case plc, ok = <-inChan:
			if !ok {
				break
			}
			this.count++
			plc.Pack.Recycle()
		case <-ticker:
			this.tally()
		}
	}
	return
}

func (this *CounterFilter) tally() {
	msgsSent := this.count - this.lastCount
	if msgsSent == 0 {
		return
	}

	now := time.Now()
	elapsedTime := now.Sub(this.lastTime)
	this.lastCount = this.count
	this.lastTime = now
	this.rate = float64(msgsSent) / elapsedTime.Seconds()
	this.rates = append(this.rates, this.rate)

	outMsg := MessageGenerator.Retrieve()
	outMsg.Message.SetType("heka.counter_output")
	outMsg.Message.SetPayload(fmt.Sprintf("Got %d messages. %0.2f msg/sec",
		this.count, this.rate))
	MessageGenerator.Inject(outMsg)

	samples := len(this.rates)
	if samples == 10 { // generate a summary every 10 samples
		sort.Float64s(this.rates)
		min := this.rates[0]
		max := this.rates[samples-1]
		sum := float64(0)
		for _, val := range this.rates {
			sum += val
		}
		mean := sum / float64(samples)
		outMsg = MessageGenerator.Retrieve()
		outMsg.Message.SetType("heka.counter_output")
		outMsg.Message.SetPayload(
			fmt.Sprintf("AGG Sum. Min: %0.2f    Max: %0.2f    Mean: %0.2f",
				min, max, mean))
		MessageGenerator.Inject(outMsg)
		this.rates = this.rates[:0]
	}
}

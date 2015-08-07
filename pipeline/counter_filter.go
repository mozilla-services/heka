/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
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
	"time"
)

// Filter that counts the number of messages flowing through and provides
// primitive aggregation counts.
type CounterFilter struct {
	lastTime  time.Time
	lastCount uint
	count     uint
	rate      float64
	rates     []float64
}

// CounterFilter config struct, used only for specifying default ticker
// interval and message matcher values.
type CounterFilterConfig struct {
	// Defaults to counting everything except the counter's own output
	// messages.
	MessageMatcher string `toml:"message_matcher"`
	// Defaults to 5 second intervals.
	TickerInterval uint `toml:"ticker_interval"`
}

func (this *CounterFilter) ConfigStruct() interface{} {
	return &CounterFilterConfig{
		MessageMatcher: "Type != 'heka.counter-output'",
		TickerInterval: uint(5),
	}
}

func (this *CounterFilter) Init(config interface{}) error {
	return nil
}

func (this *CounterFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	inChan := fr.InChan()
	ticker := fr.Ticker()
	this.lastTime = time.Now()

	var (
		ok           = true
		pack         *PipelinePack
		msgLoopCount uint
	)
	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			msgLoopCount = pack.MsgLoopCount
			this.count++
			fr.UpdateCursor(pack.QueueCursor)
			pack.Recycle(nil)
		case <-ticker:
			this.tally(fr, h, msgLoopCount)
		}
	}
	return
}

func (this *CounterFilter) CleanupForRestart() {
	this.lastCount = 0
	this.count = 0
	this.rate = 0
	this.rates = nil
}

func (this *CounterFilter) tally(fr FilterRunner, h PluginHelper,
	msgLoopCount uint) {
	const msgType = "heka.counter-output"

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

	pack, e := h.PipelinePack(msgLoopCount)
	if e != nil {
		fr.LogError(e)
		return
	}
	pack.Message.SetLogger(fr.Name())
	pack.Message.SetType(msgType)
	pack.Message.SetPayload(fmt.Sprintf("Got %d messages. %0.2f msg/sec",
		this.count, this.rate))
	fr.Inject(pack)

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
		pack, e := h.PipelinePack(msgLoopCount)
		if e != nil {
			fr.LogError(e)
			return
		}
		pack.Message.SetLogger(fr.Name())
		pack.Message.SetType(msgType)
		pack.Message.SetPayload(
			fmt.Sprintf("AGG Sum. Min: %0.2f    Max: %0.2f    Mean: %0.2f",
				min, max, mean))
		fr.Inject(pack)
		this.rates = this.rates[:0]
	}
}

func init() {
	RegisterPlugin("CounterFilter", func() interface{} {
		return new(CounterFilter)
	})
}

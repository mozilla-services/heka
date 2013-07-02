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

// Heka PluginRunner interface for Filter type plugins.
type FilterRunner interface {
	PluginRunner
	// Input channel on which the Filter should listen for incoming messages
	// to be processed. Closure of the channel signals shutdown to the filter.
	InChan() chan *PipelinePack
	// Associated Filter plugin object.
	Filter() Filter
	// Starts the Filter (so it's listening on the input channel for messages
	// to be processed) in a separate goroutine and returns. Should decrement
	// the wait group when the Filter has stopped and the goroutine has
	// completed.
	Start(h PluginHelper, wg *sync.WaitGroup) (err error)
	// Returns a ticker channel configured to send ticks at an interval
	// specified by the plugin's ticker_interval config value, if provided.
	Ticker() (ticker <-chan time.Time)
	// Hands provided PipelinePack to the Heka Router for delivery to any
	// Filter or Output plugins with a corresponding message_matcher. Returns
	// false and doesn't perform message injection if the message would be
	// caught by the sending Filter's message_matcher.
	Inject(pack *PipelinePack) bool
	// Parsing engine for this Filter's message_matcher.
	MatchRunner() *MatchRunner
	// Retains a pack for future delivery to the plugin when a plugin needs
	// to shut down and wants to retain the pack for the next time its
	// running properly
	RetainPack(pack *PipelinePack)
}

// Heka Filter plugin type.
type Filter interface {
	// Starts the filter listening on the FilterRunner's provided input
	// channel. Should not return until shutdown, signaled to the Filter by
	// the closure of the input channel. Should return a non-nil error value
	// only if errors happen during start-up or if there is an unclean
	// shutdown (i.e. not due to an error processing an isolated message, in
	// that case use FilterRunner.LogError).
	Run(r FilterRunner, h PluginHelper) (err error)
}

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
			pack.Recycle()
		case <-ticker:
			this.tally(fr, h, msgLoopCount)
		}
	}
	return
}

func (this *CounterFilter) tally(fr FilterRunner, h PluginHelper,
	msgLoopCount uint) {
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

	pack := h.PipelinePack(msgLoopCount)
	if pack == nil {
		fr.LogError(fmt.Errorf("exceeded MaxMsgLoops = %d",
			Globals().MaxMsgLoops))
		return
	}
	pack.Message.SetType("heka.counter-output")
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
		pack := h.PipelinePack(msgLoopCount)
		if pack == nil {
			fr.LogError(fmt.Errorf("exceeded MaxMsgLoops = %d",
				Globals().MaxMsgLoops))
			return
		}
		pack.Message.SetType("heka.counter-output")
		pack.Message.SetPayload(
			fmt.Sprintf("AGG Sum. Min: %0.2f    Max: %0.2f    Mean: %0.2f",
				min, max, mean))
		fr.Inject(pack)
		this.rates = this.rates[:0]
	}
}

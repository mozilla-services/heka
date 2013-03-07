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
	"github.com/mozilla-services/heka/message"
	"log"
	"runtime"
	"sort"
	"sync"
	//   "sync/atomic"
	"time"
)

type Filter interface {
	// @todo Message and Inject should allow more then just a payload string
	SetOutput(f func(s string))
	SetInjectMessage(f func(s string))
	ProcessMessage(msg *message.Message) int
	TimerEvent() int
	Destroy()
}

func defaultOutput(s string) {
	log.Println(s)
}

func defaultInjectMessage(s string) {
	log.Println(s)
}

type FilterRunner interface {
	PluginRunnerBase
	Start(helper PluginHelper, wg *sync.WaitGroup)
	Stop()
}

type filterRunner struct {
	pluginRunnerBase
	messageMatcher *message.MatcherSpecification
	outputs        []OutputRunner

	output_timer uint
	ticker       *time.Ticker
	filter       Filter
}

func (this *filterRunner) Start(helper PluginHelper, wg *sync.WaitGroup) {
	if this.ticker != nil {
		log.Printf("Attempted to start a running FilterRunner")
		wg.Done()
		return
	}

	//	this.filter.SetOutput(func(s string) {
	//		msg := MessageGenerator.Retrieve()
	//		msg.Message.SetType("heka_filter")
	//		msg.Message.SetLogger(this.name)
	//		msg.Message.SetPayload(s)
	//
	//		packSupply := helper.PackSupply()
	//		pack := <-packSupply
	//		msg.Message.Copy(pack.Message)
	//		pack.Decoded = true
	//		MessageGenerator.RecycleChan <- msg
	//	    for _, output := range this.outputs {
	//	    	atomic.AddInt32(&pack.RefCount, 1)
	//	    	output.InChan() <- pack
	//	    	pack.Recycle()
	//	    }
	//	})

	//	this.filter.SetInjectMessage(func(s string) {
	//		msg := MessageGenerator.Retrieve()
	//		msg.Message.SetType("heka_filter")
	//		msg.Message.SetLogger(this.name)
	//		msg.Message.SetPayload(s)
	//		MessageGenerator.Inject(msg)
	//	})

	this.ticker = time.NewTicker(time.Duration(this.output_timer) * time.Second)
	go func() {
		log.Printf("Filter started: %s\n", this.Name())
		var pack *PipelinePack
		var ok bool = true
		for ok {
			runtime.Gosched()
			select {
			case pack, ok = <-this.InChan():
				if !ok {
					break
				}
				if this.messageMatcher != nil &&
					this.messageMatcher.IsMatch(pack.Message) {
					r := this.filter.ProcessMessage(pack.Message)
					if r != 0 {
						log.Printf("%s - ProcessMessage returned %d", this.name, r)
					}
				}
				pack.Recycle()
			case <-this.ticker.C:
				r := this.filter.TimerEvent()
				if r != 0 {
					log.Printf("%s - TimerEvent returned %d", this.name, r)
				}
			}
		}
		this.ticker.Stop()
		this.ticker = nil
		this.filter.Destroy()
		log.Printf("Filter stopped: %s\n", this.Name())
		wg.Done()
	}()
}

func (this *filterRunner) Stop() {
	close(this.InChan())
}

type CounterFilter struct {
	lastTime      time.Time
	lastCount     uint
	count         uint
	zeroes        int8
	rate          float64
	rates         []float64
	intervals     uint
	output        func(s string)
	injectMessage func(s string)
}

func (this *CounterFilter) Init(config interface{}) error {
	this.lastTime = time.Now()
	this.output = defaultOutput
	this.injectMessage = defaultInjectMessage
	return nil
}

func (this *CounterFilter) Destroy() {
}

func (this *CounterFilter) SetOutput(f func(s string)) {
	this.output = f
}

func (this *CounterFilter) SetInjectMessage(f func(s string)) {
	this.injectMessage = f
}

func (this *CounterFilter) ProcessMessage(msg *message.Message) int {
	this.count++
	return 0
}

func (this *CounterFilter) TimerEvent() int {
	this.intervals++
	now := time.Now()
	msgsSent := this.count - this.lastCount
	elapsedTime := now.Sub(this.lastTime)
	this.lastCount = this.count
	this.lastTime = now
	this.rate = float64(msgsSent) / elapsedTime.Seconds()
	if msgsSent == 0 {
		if msgsSent == 0 || this.zeroes == 3 {
			return 0
		}
		this.zeroes++
	} else {
		this.zeroes = 0
	}
	this.output(fmt.Sprintf("Got %d messages. %0.2f msg/sec", this.count,
		this.rate))
	this.rates = append(this.rates, this.rate)
	if this.intervals >= 10 {
		this.intervals = 0
		amount := len(this.rates)
		if amount < 1 {
			return 0
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
		this.output(fmt.Sprintf("AGG Sum. Min: %0.2f    Max: %0.2f    Mean: %0.2f",
			min, max, mean))
		this.rates = this.rates[:0]
	}
	return 0
}

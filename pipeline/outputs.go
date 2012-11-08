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
	"github.com/crankycoder/g2s"
	"log"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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
}

type CounterGlobal struct {
	count chan uint
	once  sync.Once
}

var counterGlobal CounterGlobal

func InitCountChan() {
	counterGlobal.count = make(chan uint, 30000)
	go timerLoop()
}

func (self *CounterOutput) Init(config interface{}) error {
	counterGlobal.once.Do(InitCountChan)
	return nil
}

func (self *CounterOutput) Deliver(pipelinePack *PipelinePack) {
	counterGlobal.count <- 1
}

func timerLoop() {
	tick := time.NewTicker(time.Duration(time.Second))
	aggregate := time.NewTicker(time.Duration(10 * time.Second))
	lastTime := time.Now()
	lastCount := uint(0)
	count := uint(0)
	zeroes := int8(0)
	var (
		msgsSent, inc uint
		elapsedTime   time.Duration
		now           time.Time
		rate          float64
		rates         []float64
	)
	for {
		// Here for performance reasons
		runtime.Gosched()
		select {
		case <-aggregate.C:
			amount := len(rates)
			if amount < 1 {
				continue
			}
			sort.Float64s(rates)
			min := rates[0]
			max := rates[amount-1]
			mean := min
			sum := float64(0)
			for _, val := range rates {
				sum += val
			}
			mean = sum / float64(amount)
			log.Printf("AGG Sum. Min: %0.2f   Max: %0.2f     Mean: %0.2f",
				min, max, mean)
			rates = rates[:0]
		case <-tick.C:
			now = time.Now()
			msgsSent = count - lastCount
			lastCount = count
			elapsedTime = now.Sub(lastTime)
			lastTime = now
			rate = float64(msgsSent) / elapsedTime.Seconds()
			if msgsSent == 0 {
				if msgsSent == 0 || zeroes == 3 {
					continue
				}
				zeroes++
			} else {
				zeroes = 0
			}
			log.Printf("Got %d messages. %0.2f msg/sec\n", count, rate)
			rates = append(rates, rate)
		case inc = <-counterGlobal.count:
			count += inc
		}
	}
}

type StatsdOutput struct {
	statsdClient StatsdClient
}

func NewStatsdClient(url string) StatsdClient {
	sd, err := g2s.NewStatsd(url, 0)
	if err != nil {
		log.Printf("Error!! No statsd client was created! %v", err)
	}
	return sd
}

func (self *StatsdOutput) Init(config interface{}) error {
	statsd_url := (*config.(*PluginConfig))["url"].(string)
	self.statsdClient = NewStatsdClient(statsd_url)
	return nil
}

func (self *StatsdOutput) Deliver(pipelinePack *PipelinePack) {

	msg := pipelinePack.Message

	// we need the ns for the full key
	ns := msg.Logger

	key := msg.Fields["name"].(string)
	if strings.TrimSpace(ns) != "" {
		s := []string{ns, key}
		key = strings.Join(s, ".")
	}

	tmp_value, value_err := strconv.ParseInt(msg.Payload, 10, 32)
	if value_err != nil {
		log.Printf("Error parsing value for statsd")
		return
	}
	// Downcast this
	value := int(tmp_value)

	rate := float32(msg.Fields["rate"].(float64))

	switch msg.Type {
	case "counter":
		self.statsdClient.IncrementSampledCounter(key, value, rate)
	case "timer":
		self.statsdClient.SendSampledTiming(key, value, rate)
    //default:
//		log.Printf("Warning: Unexpected event passed into StatsdOutput.\nEvent => %+v\n", *(pipelinePack.Message))
	}
	runtime.Gosched()
}

func NewStatsdOutput(statsdClient StatsdClient) *StatsdOutput {
	self := StatsdOutput{}
	return &self
}

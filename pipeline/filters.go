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
	"bytes"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Filter interface {
	Plugin
	FilterMsg(pipelinePack *PipelinePack)
}

type StatPacket struct {
	Bucket   string
	Value    int
	Modifier string
	Sampling float32
}

// StatFilter global
type statFilter struct {
	flushInterval    int64
	percentThreshold int
	StatsIn          chan *StatPacket
	recycleChan      chan *StatPacket
	counters         map[string]int
	timers           map[string][]int
	gauges           map[string]int
	once             sync.Once
	p                *StatPacket
}

var StatFilter *statFilter = new(statFilter)

// Initialize the StatFilter global
//
// This must be called only once, using the StatFilter.once.Do.
func (self *statFilter) Init(flushInterval int64, percentThreshold int) {
	log.Println("Made ithere")
	self.flushInterval = flushInterval
	self.percentThreshold = percentThreshold
	self.StatsIn = make(chan *StatPacket, PoolSize)
	self.recycleChan = make(chan *StatPacket, PoolSize)
	self.counters = make(map[string]int)
	self.timers = make(map[string][]int)
	self.gauges = make(map[string]int)
	for i := 0; i < PoolSize; i++ {
		self.recycleChan <- new(StatPacket)
	}
	go self.Monitor()
}

// StatFilter Monitor pulls StatPackets off the StatsIn channel,
// updates the internal stat counters then recycles the StatPacket
func (self *statFilter) Monitor() {
	t := time.NewTicker(time.Duration(self.flushInterval) * time.Second)
	for {
		// Yielding before a channel select improves scheduler performance
		runtime.Gosched()
		select {
		case <-t.C:
			self.Flush()
		case self.p = <-self.StatsIn:
			if self.p.Modifier == "ms" {
				_, ok := self.timers[self.p.Bucket]
				if !ok {
					var t []int
					self.timers[self.p.Bucket] = t
				}
				self.timers[self.p.Bucket] = append(self.timers[self.p.Bucket], self.p.Value)
			} else if self.p.Modifier == "g" {
				_, ok := self.gauges[self.p.Bucket]
				if !ok {
					self.gauges[self.p.Bucket] = 0
				}
				self.gauges[self.p.Bucket] += self.p.Value
			} else {
				_, ok := self.counters[self.p.Bucket]
				if !ok {
					self.counters[self.p.Bucket] = 0
				}
				self.counters[self.p.Bucket] += int(float32(self.p.Value) * (1 / self.p.Sampling))
			}
			self.recycleChan <- self.p
		}
	}
}

// statFilter Flush is called every flushInterval seconds to flush a
// the aggregated stats as a statmetric injected message
func (self *statFilter) Flush() {
	var value int64
	numStats := 0
	now := time.Now()
	buffer := bytes.NewBufferString("")
	for s, c := range self.counters {
		value = int64(c) / ((self.flushInterval * int64(time.Second)) / 1e3)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", s, value, now)
		fmt.Fprintf(buffer, "stats_counts.%s %d %d\n", s, c, now)
		self.counters[s] = 0
		numStats++
	}
	for i, g := range self.gauges {
		value = int64(g)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", i, value, now)
		numStats++
	}
	var count, min, max, mean, maxAtThreshold, thresholdIndex, numInThreshold, sum, i int
	var values []int
	for u, t := range self.timers {
		if len(t) > 0 {
			sort.Ints(t)
			min = t[0]
			max = t[len(t)-1]
			mean = min
			maxAtThreshold = max
			count = len(t)
			if len(t) > 1 {
				thresholdIndex = ((100 - self.percentThreshold) / 100) * count
				numInThreshold = count - thresholdIndex
				values = t[0:numInThreshold]

				sum = 0
				for i = 0; i < numInThreshold; i++ {
					sum += values[i]
				}
				mean = sum / numInThreshold
			}
			self.timers[u] = t[:0]

			fmt.Fprintf(buffer, "stats.timers.%s.mean %d %d\n", u, mean, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", u, max, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %d %d\n", u,
				self.percentThreshold, maxAtThreshold, now)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", u, min, now)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, count, now)
		}
		numStats++
	}
	fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, now)

	if numStats == 0 {
		log.Println("No stats collected, not delivering.")
	}
	msgHolder := MessageGenerator.Retrieve(0)
	msgHolder.Message.Type = "statmetric"
	msgHolder.Message.Timestamp = now
	msgHolder.Message.Payload = buffer.String()
	MessageGenerator.Inject(msgHolder)
}

// StatRollupFilterConfig is used to specify the config vars this takes
type StatRollupFilterConfig struct {
	FlushInterval    int64
	PercentThreshold int
}

func (self *StatRollupFilter) ConfigStruct() interface{} {
	return &StatRollupFilterConfig{FlushInterval: 10, PercentThreshold: 90}
}

// StatRollupFilter is created per pipelinePack
type StatRollupFilter struct {
	packet *StatPacket
}

func (self *StatRollupFilter) Init(config interface{}) (err error) {
	StatFilter.once.Do(func() {
		conf := config.(*StatRollupFilterConfig)
		StatFilter.Init(conf.FlushInterval, conf.PercentThreshold)
	})
	return nil
}

func (self *StatRollupFilter) FilterMsg(pipeline *PipelinePack) {
	self.packet = <-StatFilter.recycleChan
	switch pipeline.Message.Type {
	case "statsd_timer":
		self.packet.Modifier = "ms"
	case "statsd_gauge":
		self.packet.Modifier = "g"
	case "statsd_counter":
		self.packet.Modifier = ""
	default:
		return
	}

	defer func() {
		pipeline.Blocked = true
	}()

	self.packet.Bucket = pipeline.Message.Fields["name"].(string)
	value, err := strconv.ParseInt(pipeline.Message.Payload, 0, 0)
	if err != nil {
		log.Println("StatRollupFilter error parsing value: %s", err.Error())
		return
	}
	self.packet.Value = int(value)
	self.packet.Sampling = pipeline.Message.Fields["rate"].(float32)
	StatFilter.StatsIn <- self.packet
}

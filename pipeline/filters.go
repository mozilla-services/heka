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
	"sort"
	"strconv"
	"sync"
	"time"
)

type Filter interface {
	Plugin
	FilterMsg(pipelinePack *PipelinePack)
}

// StatRollupFilter
type Packet struct {
	Bucket   string
	Value    int
	Modifier string
	Sampling float32
}

// StatRollupFilter is created per pipelinePack
type StatRollupFilter struct {
}

// StatRollupFilterConfig is used to specify the config vars this takes
type StatRollupFilterConfig struct {
	FlushInterval    int64
	PercentThreshold int
}

// StatRollupFilterGlobal is used for the statRollupGlobal object that
// collects the stats from all the StatRollupFilter instances
type StatRollupFilterGlobal struct {
	flushInterval    int64
	percentThreshold int
	StatsIn          chan *Packet
	counters         map[string]int
	timers           map[string][]int
	gauges           map[string]int
	messageGenerator *MessageGeneratorInput
	once             sync.Once
}

// A single StatRollup global instance for use by the statrollup
// filters
var statRollupGlobal StatRollupFilterGlobal

func SetupStatConfig(config interface{}) {
	conf := config.(*StatRollupFilterConfig)
	statRollupGlobal.flushInterval = conf.FlushInterval
	statRollupGlobal.percentThreshold = conf.PercentThreshold
	statRollupGlobal.StatsIn = make(chan *Packet, 10000)
	statRollupGlobal.counters = make(map[string]int)
	statRollupGlobal.timers = make(map[string][]int)
	statRollupGlobal.gauges = make(map[string]int)
	go statRollupGlobal.Monitor()
}

func (self *StatRollupFilter) ConfigStruct() interface{} {
	return &StatRollupFilterConfig{FlushInterval: 10, PercentThreshold: 90}
}

func (self *StatRollupFilter) Init(config interface{}) (err error) {
	statRollupGlobal.once.Do(func() { SetupStatConfig(config) })
	return nil
}

func (self *StatRollupFilter) FilterMsg(pipeline *PipelinePack) {
	var packet Packet
	msg := pipeline.Message
	switch msg.Type {
	case "statsd_timer":
		packet.Modifier = "ms"
	case "statsd_gauge":
		packet.Modifier = "g"
	case "statsd_counter":
		packet.Modifier = ""
	default:
		return
	}

	defer func() {
		pipeline.Blocked = true
	}()

	packet.Bucket = msg.Fields["name"].(string)
	value, err := strconv.ParseInt(msg.Payload, 0, 0)
	if err != nil {
		log.Println("StatRollupFilter error parsing value: %s", err.Error())
		return
	}
	packet.Value = int(value)
	packet.Sampling = msg.Fields["rate"].(float32)
	statRollupGlobal.StatsIn <- &packet
}

func (self *StatRollupFilterGlobal) Monitor() {
	t := time.NewTicker(time.Duration(self.flushInterval) * time.Second)
	for {
		select {
		case <-t.C:
			self.Flush()
		case s := <-self.StatsIn:
			if s.Modifier == "ms" {
				_, ok := self.timers[s.Bucket]
				if !ok {
					var t []int
					self.timers[s.Bucket] = t
				}
				self.timers[s.Bucket] = append(self.timers[s.Bucket], s.Value)
			} else if s.Modifier == "g" {
				_, ok := self.gauges[s.Bucket]
				if !ok {
					self.gauges[s.Bucket] = 0
				}
				self.gauges[s.Bucket] += s.Value
			} else {
				_, ok := self.counters[s.Bucket]
				if !ok {
					self.counters[s.Bucket] = 0
				}
				self.counters[s.Bucket] += int(float32(s.Value) * (1 / s.Sampling))
			}
		}
	}
}

func (self *StatRollupFilterGlobal) Flush() {
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
	msgHolder := MessageGenerator.Retrieve()
	msgHolder.Message.Type = "statmetric"
	msgHolder.Message.Timestamp = now
	msgHolder.Message.Payload = buffer.String()
	MessageGenerator.Inject(msgHolder)
}

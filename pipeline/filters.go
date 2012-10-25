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
	"time"
)

type Filter interface {
	Plugin
	FilterMsg(pipelinePack *PipelinePack)
}

// LogFilter
type LogFilter struct {
}

func (self *LogFilter) Init(config *PluginConfig) error {
	return nil
}

func (self *LogFilter) FilterMsg(pipelinePack *PipelinePack) {
	log.Printf("Message: %+v\n", pipelinePack.Message)
}

// NamedOutputFilter
type NamedOutputFilter struct {
	outputNames []string
}

func NewNamedOutputFilter(outputNames []string) *NamedOutputFilter {
	self := NamedOutputFilter{outputNames}
	return &self
}

func (self *NamedOutputFilter) Init(config *PluginConfig) error {
	return nil
}

func (self *NamedOutputFilter) FilterMsg(pipelinePack *PipelinePack) {
	for _, outputName := range self.outputNames {
		pipelinePack.Outputs[outputName] = true
	}
}

func (self *StatRollupFilter) Init(config *FilterConfig) (err error) {
	self.flushInterval = int64(config.FlushInterval)
	self.percentThreshold = int(config.PercentThreshold)
	self.StatdIn = make(chan Packet, 10000)
// StatRollupFilter
type Packet struct {
	Bucket   string
	Value    int
	Modifier string
	Sampling float32
}

type StatRollupFilter struct {
	flushInterval    int64
	percentThreshold int
	StatsIn          chan *Packet
	counters         map[string]int
	timers           map[string][]int
	gauges           map[string]int
}
	self.counters = make(map[string]int)
	self.timers = make(map[string][]int)
	self.gauges = make(map[string]int)
	go self.Monitor()
	return nil
}

func (self *StatRollupFilter) Monitor() {
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

func (self *StatRollupFilter) Flush() {
	numStats := 0
	now := time.Now()
	buffer := bytes.NewBufferString("")
	for s, c := range self.counters {
		value := int64(c) / ((self.flushInterval * int64(time.Second)) / 1e3)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", s, value, now)
		fmt.Fprintf(buffer, "stats_counts.%s %d %d\n", s, c, now)
		self.counters[s] = 0
		numStats++
	}
	for i, g := range self.gauges {
		value := int64(g)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", i, value, now)
		numStats++
	}
	for u, t := range self.timers {
		if len(t) > 0 {
			sort.Ints(t)
			min := t[0]
			max := t[len(t)-1]
			mean := min
			maxAtThreshold := max
			count := len(t)
			if len(t) > 1 {
				var thresholdIndex int
				thresholdIndex = ((100 - self.percentThreshold) / 100) * count
				numInThreshold := count - thresholdIndex
				values := t[0:numInThreshold]

				sum := 0
				for i := 0; i < numInThreshold; i++ {
					sum += values[i]
				}
				mean = sum / numInThreshold
			}
			var z []int
			self.timers[u] = z

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
	fmt.Println(buffer)
}

func (self *StatRollupFilter) FilterMsg(pipeline *PipelinePack) {
	var packet Packet
	msg := *pipeline.Message
	switch msg.Type {
	case "timer":
		packet.Modifier = "ms"
	case "gauge":
		packet.Modifier = "g"
	case "counter":
		packet.Modifier = ""
	default:
		return
	}
	packet.Bucket = msg.Fields["name"]
	packet.Value = int(msg.Payload)
	packet.Sampling = float32(msg.Fields["rate"])
	self.StatsIn <- &packet
	pipeline.Message = nil
}

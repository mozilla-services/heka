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
	"errors"
	"fmt"
	. "heka/message"
	"log"
	"sort"
	"strconv"
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

// StatRollupFilter
type Packet struct {
	Bucket   string
	Value    int
	Modifier string
	Sampling float32
}

// A local statsd like structure that rolls-up counter/timer/gauge type
// messages and later creates a StatMetric type message which is
// inserted via the MessageGeneratorInput
type StatRollupFilter struct {
	messageGenerator *Plugin
	flushInterval    int64
	percentThreshold int
	StatsIn          chan *Packet
	counters         map[string]int
	timers           map[string][]int
	gauges           map[string]int
}

func (self *StatRollupFilter) Init(config *PluginConfig) (err error) {
	var ok bool
	var value interface{}
	value, ok = (*config)["FlushInterval"]
	if !ok {
		return errors.New("StatRollupFilter config: Missing FlushInterval")
	}
	self.flushInterval = value.(int64)
	value, ok = (*config)["PercentThreshold"]
	if !ok {
		return errors.New("StatRollupFilter config: Missing PercentThreshold")
	}
	self.percentThreshold = value.(int)
	self.StatsIn = make(chan *Packet, 10000)
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

	fmt.Println(buffer) // Prints to std out just for fun
}

// Scans the config to locate the MessageGeneratorInput and saves a
// reference to it for use when filtering messages
func (self *StatRollupFilter) SetupMessageGenerator(config *GraterConfig) bool {
	for name, output := range config.Outputs {
		convert, ok := output.(MessageGeneratorInput)
		if ok {
			self.messageGenerator = output
			return true
		}
	}
	return false
}

func (self *StatRollupFilter) FilterMsg(pipeline *PipelinePack) {
	// If there's an message generator input, configure it. This has to
	// be setup during run-time as the inputs aren't setup or during
	// filter initialization. Filtering will *not* occur if no message
	// generator is found
	if self.messageGenerator == nil {
		ok := self.SetupMessageGenerator(pipeline.Config)
		if !ok {
			return
		}
	}
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
		pipeline.Message = nil
	}()

	packet.Bucket = msg.Fields["name"].(string)
	value, err := strconv.ParseInt(msg.Payload, 0, 0)
	if err != nil {
		log.Println("StatRollupFilter error parsing value: %s", err.Error())
		return
	}
	packet.Value = int(value)
	packet.Sampling = msg.Fields["rate"].(float32)
	self.StatsIn <- &packet
}

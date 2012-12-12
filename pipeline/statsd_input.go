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
#   Ben Bangert (bbangert@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"time"
)

var (
	sanitizeRegexp = regexp.MustCompile("[^a-zA-Z0-9\\-_\\.:\\|@]")
	packetRegexp   = regexp.MustCompile("([a-zA-Z0-9_]+):(\\-?[0-9\\.]+)\\|(c|ms)(\\|@([0-9\\.]+))?")
)

type StatPacket struct {
	Bucket   string
	Value    string
	Modifier string
	Sampling float32
}

type StatsdUdpInputConfig struct {
	Address          string
	FlushInterval    int64
	PercentThreshold int
}

func (self *StatsdUdpInput) ConfigStruct() interface{} {
	return &StatsdUdpInputConfig{FlushInterval: 10, PercentThreshold: 90}
}

// Deadline is stored on the struct so we don't have to allocate / GC
// a new time.Time object for each message received.
type StatsdUdpInput struct {
	Listener    net.Conn
	Deadline    time.Time
	StatsIn     chan *StatPacket
	recycleChan chan *StatPacket
	p           *StatPacket
	value       string
}

func (self *StatsdUdpInput) InitOnce(config interface{}) (global PluginGlobal, err error) {
	conf := config.(*StatsdUdpInputConfig)
	stat := new(statsUdp)
	stat.FlushInterval = conf.FlushInterval
	stat.PercentThreshold = conf.PercentThreshold
	stat.StatsIn = make(chan *StatPacket, PoolSize*2)
	stat.recycleChan = make(chan *StatPacket, PoolSize*2)
	stat.counters = make(map[string]int)
	stat.timers = make(map[string][]float64)
	stat.gauges = make(map[string]int)
	for i := 0; i < PoolSize; i++ {
		stat.recycleChan <- new(StatPacket)
	}
	go stat.Monitor()
	return stat, nil
}

// Unused Init to meet Plugin interface
func (self *StatsdUdpInput) Init(config interface{}) error { return nil }

func (self *StatsdUdpInput) InitWithGlobal(global PluginGlobal, config interface{}) error {
	conf := config.(*StatsdUdpInputConfig)
	stat := global.(*statsUdp)
	self.StatsIn = stat.StatsIn
	self.recycleChan = stat.recycleChan
	if len(conf.Address) > 3 && conf.Address[:3] == "fd:" {
		// File descriptor
		fdStr := conf.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			log.Println(err)
			return fmt.Errorf("Invalid file descriptor: %s", conf.Address)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		self.Listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		self.Listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	return nil
}

func (self *StatsdUdpInput) Read(pipelinePack *PipelinePack,
	timeout *time.Duration) error {
	pipelinePack.Blocked = true
	self.Deadline = time.Now().Add(*timeout)
	self.Listener.SetReadDeadline(self.Deadline)
	n, err := self.Listener.Read(pipelinePack.MsgBytes)
	if err == nil {
		pipelinePack.MsgBytes = pipelinePack.MsgBytes[:n]
		s := sanitizeRegexp.ReplaceAllString(string(pipelinePack.MsgBytes), "")
		for _, item := range packetRegexp.FindAllStringSubmatch(s, -1) {
			self.p = <-self.recycleChan
			self.value = item[2]
			if item[3] == "ms" {
				_, err := strconv.ParseFloat(item[2], 32)
				if err != nil {
					self.value = "0"
				}
			}

			sampleRate, err := strconv.ParseFloat(item[5], 32)
			if err != nil {
				sampleRate = 1
			}

			self.p.Bucket = item[1]
			self.p.Value = self.value
			self.p.Modifier = item[3]
			self.p.Sampling = float32(sampleRate)

			self.StatsIn <- self.p
		}
	}
	return err
}

type statsUdp struct {
	FlushInterval    int64
	PercentThreshold int
	StatsIn          chan *StatPacket
	recycleChan      chan *StatPacket
	counters         map[string]int
	timers           map[string][]float64
	gauges           map[string]int
	p                *StatPacket
}

func (self *statsUdp) Event(eventType string) {
}

// StatsdUdpInput Monitor pulls StatPackets off the StatsIn channel,
// updates the internal stat counters then recycles the StatPacket
func (self *statsUdp) Monitor() {
	t := time.NewTicker(time.Duration(self.FlushInterval) * time.Second)
	var floatValue float64
	var intValue int
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
					var t []float64
					self.timers[self.p.Bucket] = t
				}
				floatValue, _ = strconv.ParseFloat(self.p.Value, 64)
				self.timers[self.p.Bucket] = append(self.timers[self.p.Bucket], floatValue)
			} else if self.p.Modifier == "g" {
				_, ok := self.gauges[self.p.Bucket]
				if !ok {
					self.gauges[self.p.Bucket] = 0
				}
				intValue, _ = strconv.Atoi(self.p.Value)
				self.gauges[self.p.Bucket] += intValue
			} else {
				_, ok := self.counters[self.p.Bucket]
				if !ok {
					self.counters[self.p.Bucket] = 0
				}
				floatValue, _ = strconv.ParseFloat(self.p.Value, 32)
				self.counters[self.p.Bucket] += int(float32(floatValue) * (1 / self.p.Sampling))
			}
			self.recycleChan <- self.p
		}
	}
}

// statsUdp Flush is called every flushInterval seconds to flush a
// the aggregated stats as a statmetric injected message
func (self *statsUdp) Flush() {
	var value float64
	var intval int64
	numStats := 0
	now := time.Now()
	buffer := bytes.NewBufferString("")
	for s, c := range self.counters {
		value = float64(c) / ((float64(self.FlushInterval) * float64(time.Second)) / float64(1e3))
		fmt.Fprintf(buffer, "stats.%s %d %d\n", s, value, now)
		fmt.Fprintf(buffer, "stats_counts.%s %d %d\n", s, c, now)
		self.counters[s] = 0
		numStats++
	}
	for i, g := range self.gauges {
		intval = int64(g)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", i, intval, now)
		numStats++
	}
	var min, max, mean, maxAtThreshold, sum float64
	var count, thresholdIndex, numInThreshold, i int
	var values []float64
	for u, t := range self.timers {
		if len(t) > 0 {
			sort.Float64s(t)
			min = t[0]
			max = t[len(t)-1]
			mean = min
			maxAtThreshold = max
			count = len(t)
			if len(t) > 1 {
				thresholdIndex = ((100 - self.PercentThreshold) / 100) * count
				numInThreshold = count - thresholdIndex
				values = t[0:numInThreshold]

				sum = float64(0)
				for i = 0; i < numInThreshold; i++ {
					sum += values[i]
				}
				mean = sum / float64(numInThreshold)
			}
			self.timers[u] = t[:0]

			fmt.Fprintf(buffer, "stats.timers.%s.mean %d %d\n", u, mean, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", u, max, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %d %d\n", u,
				self.PercentThreshold, maxAtThreshold, now)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", u, min, now)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, count, now)
		}
		numStats++
	}
	fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, now)
	msgHolder := MessageGenerator.Retrieve(0)
	msgHolder.Message.Type = "statmetric"
	msgHolder.Message.Timestamp = now
	msgHolder.Message.Payload = buffer.String()
	MessageGenerator.Inject(msgHolder)
}

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
	"sort"
	"strconv"
	"time"
)

var (
	sanitizeRegexp = regexp.MustCompile("[^a-zA-Z0-9\\-_\\.:\\|@]")
	packetRegexp   = regexp.MustCompile("([a-zA-Z0-9_]+):(\\-?[0-9\\.]+)\\|(c|ms)(\\|@([0-9\\.]+))?")
)

type StatsdUdpInputConfig struct {
	Address          string
	FlushInterval    int64
	PercentThreshold int
}

// Deadline is stored on the struct so we don't have to allocate / GC
// a new time.Time object for each message received.
type StatsdWriter struct {
	Listener         net.Conn
	Deadline         time.Time
	PercentThreshold int
	FlushInterval    int64
	counters         map[string]int
	timers           map[string][]float64
	gauges           map[string]int
	packets          []byte
	readPacket       []byte
	value            string
	p                StatPacket
}

type StatPacket struct {
	Bucket   string
	Value    string
	Modifier string
	Sampling float32
}

func (self *StatsdWriter) ConfigStruct() interface{} {
	return &StatsdUdpInputConfig{FlushInterval: 10, PercentThreshold: 90}
}

func (self *StatsdWriter) Event(eventType string) {
}

func (self *StatsdWriter) MakeOutData() interface{} {
	d := make([]byte, 2000)
	return &d
}

func (self *StatsdWriter) ZeroOutData(outData interface{}) {
	outData = (outData.([]byte))[:0]
}

func (self *StatsdWriter) Init(config interface{}) (<-chan time.Time, error) {
	conf := config.(*StatsdUdpInputConfig)
	self.FlushInterval = conf.FlushInterval
	self.PercentThreshold = conf.PercentThreshold
	self.counters = make(map[string]int)
	self.timers = make(map[string][]float64)
	self.gauges = make(map[string]int)
	self.p = StatPacket{}

	if len(conf.Address) > 3 && conf.Address[:3] == "fd:" {
		// File descriptor
		fdStr := conf.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			log.Println(err)
			return nil, fmt.Errorf("Invalid file descriptor: %s", conf.Address)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		self.Listener, err = net.FileConn(udpFile)
		if err != nil {
			return nil, fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
		if err != nil {
			return nil, fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		self.Listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	ticker := time.NewTicker(time.Duration(conf.FlushInterval) * time.Second)
	return ticker.C, nil
}

func (self *StatsdWriter) PrepOutData(pipelinePack *PipelinePack, outData interface{},
	timeout *time.Duration) error {
	pipelinePack.Blocked = true
	self.Deadline = time.Now().Add(*timeout)
	self.Listener.SetReadDeadline(self.Deadline)
	n, err := self.Listener.Read(pipelinePack.MsgBytes)
	if err == nil {
		pipelinePack.MsgBytes = pipelinePack.MsgBytes[:n]
		outData = append(outData.([]byte), pipelinePack.MsgBytes...)
	}
	return err
}

func (self *StatsdWriter) Batch(outData interface{}) (err error) {
	s := sanitizeRegexp.ReplaceAllString(string(*(outData.(*[]byte))), "")
	var sampleRate float64
	var floatValue float64
	var intValue int
	for _, item := range packetRegexp.FindAllStringSubmatch(s, -1) {
		self.p.Bucket = item[1]
		self.p.Modifier = item[3]
		self.p.Value = item[2]
		if item[3] == "ms" {
			_, err = strconv.ParseFloat(item[2], 32)
			if err != nil {
				self.p.Value = "0"
			}
		}

		sampleRate, err = strconv.ParseFloat(item[5], 32)
		if err != nil {
			sampleRate = 1
		}
		self.p.Sampling = float32(sampleRate)

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
	}
	return
}

// statsUdp Flush is called every flushInterval seconds to flush a
// the aggregated stats as a statmetric injected message
func (self *StatsdWriter) Commit() (err error) {
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
	return
}

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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"log"
	"net"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	sanitizeRegexp = regexp.MustCompile("[^a-zA-Z0-9\\-_\\.:\\|@]")
	packetRegexp   = regexp.MustCompile("([a-zA-Z0-9_]+):(\\-?[0-9\\.]+)\\|(c|ms|g)(\\|@([0-9\\.]+))?")
)

// A Heka Input plugin that handles statsd metric style input and flushes
// aggregated values. It can listen on a UDP address if configured to do so
// for standard statsd packets of message type Counter, Gauge, or Timer. It
// also accepts StatPacket objects generated from within Heka itself (usually
// via a configured StatFilter plugin) over the exposed `Packet` channel. It
// currently doesn't support Sets or other metric types.
type StatsdInput struct {
	// Channel for StatPackets, these are fed in by UDP when configured or can
	// be directly sent in from other Plugins as needed.
	Packet chan<- StatPacket

	name             string
	listener         *net.UDPConn
	percentThreshold int
	flushInterval    int64
	stopped          bool
	stopChan         chan bool
}

// StatsInput config struct
type StatsdInputConfig struct {
	// UDP Address to listen to for statsd packets. If left blank, no UDP
	// listener will be established.
	Address string
	// How frequently to flush aggregated statsd metrics in seconds. Defaults
	// to 10.
	FlushInterval int64
	// Percent threshold to use for computing "upper_N%" type stat values.
	// Defaults to 90.
	PercentThreshold int
}

// A StatPacket appropriate for a plugin to feed directly into the
// StatsdInput.Packet channel.
type StatPacket struct {
	Bucket   string
	Value    string
	Modifier string
	Sampling float32
}

func (s *StatsdInput) ConfigStruct() interface{} {
	return &StatsdInputConfig{FlushInterval: 10, PercentThreshold: 90}
}

func (s *StatsdInput) Init(config interface{}) error {
	conf := config.(*StatsdInputConfig)
	s.flushInterval = conf.FlushInterval
	s.percentThreshold = conf.PercentThreshold

	if conf.Address != "" {
		udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		s.listener, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	return nil
}

// Creates a `StatMonitor` listening on the `Packets` channel for incoming
// StatPackets, and spins up a statsd server listening on a UDP connection if
// configured to do so.
func (s *StatsdInput) Run(ir InputRunner, h PluginHelper) (err error) {
	packets := make(chan StatPacket, 5000)
	s.Packet = packets
	s.stopChan = make(chan bool)
	sm := NewStatMonitor(s.percentThreshold, s.flushInterval, ir, h)
	var wg sync.WaitGroup
	wg.Add(1)
	go sm.Monitor(packets, &wg, s.stopChan)

	// Spin up the UDP listener if it was configured
	if s.listener != nil {
		var n int
		var e error
		defer s.listener.Close()
		timeout := time.Duration(time.Millisecond * 100)

		for !s.stopped {
			message := make([]byte, 512)
			s.listener.SetReadDeadline(time.Now().Add(timeout))
			n, _, e = s.listener.ReadFromUDP(message)
			if e != nil || n == 0 {
				continue
			}
			if s.stopped {
				// If we're stopping, use synchronous call so we don't
				// close the channel too soon.
				s.handleMessage(message[:n])
			} else {
				go s.handleMessage(message[:n])
			}
		}
	}
	wg.Wait()
	return
}

func (s *StatsdInput) Stop() {
	if s.listener != nil {
		s.stopped = true
	}
	close(s.stopChan)
}

// Parses received raw statsd bytes data and converts it into a StatPacket
// object that can be passed to the StatMonitor.
func (s *StatsdInput) handleMessage(message []byte) {
	var packet StatPacket
	var value string
	st := sanitizeRegexp.ReplaceAllString(string(message), "")
	for _, item := range packetRegexp.FindAllStringSubmatch(st, -1) {
		value = item[2]
		if item[3] == "ms" {
			_, err := strconv.ParseFloat(item[2], 32)
			if err != nil {
				value = "0"
			}
		}

		sampleRate, err := strconv.ParseFloat(item[5], 32)
		if err != nil {
			sampleRate = 1
		}

		packet.Bucket = item[1]
		packet.Value = value
		packet.Modifier = item[3]
		packet.Sampling = float32(sampleRate)
		s.Packet <- packet
	}
}

// Specialized object that listens on a provided channel for StatPacket
// objects, from which it accumulates and stores statsd-style metrics data,
// periodically generating and injecting `statmetric` messages with a payload
// containing the accumulated data formatted as graphite would expect.
type statMonitor struct {
	counters         map[string]int
	timers           map[string][]float64
	gauges           map[string]int
	percentThreshold int
	flushInterval    int64
	ir               InputRunner
	h                PluginHelper
}

// Returns a new statMonitor object.
func NewStatMonitor(percentThreshold int, flushInterval int64, ir InputRunner,
	h PluginHelper) *statMonitor {
	return &statMonitor{
		counters:         make(map[string]int),
		timers:           make(map[string][]float64),
		gauges:           make(map[string]int),
		percentThreshold: percentThreshold,
		flushInterval:    flushInterval,
		ir:               ir,
		h:                h,
	}
}

// Main statMonitor loop. Doesn't return until the provided channel is closed.
// Should be run in its own goroutine.
func (sm *statMonitor) Monitor(packets <-chan StatPacket, wg *sync.WaitGroup, stopChan <-chan bool) {
	var s StatPacket
	var floatValue float64
	var intValue int

	t := time.Tick(time.Duration(sm.flushInterval) * time.Second)
	ok := true
	for ok {
		select {
		case <-stopChan:
			sm.Flush()
			ok = false
		case <-t:
			sm.Flush()
		case s = <-packets:
			switch s.Modifier {
			case "ms":
				floatValue, _ = strconv.ParseFloat(s.Value, 64)
				sm.timers[s.Bucket] = append(sm.timers[s.Bucket], floatValue)
			case "g":
				intValue, _ = strconv.Atoi(s.Value)
				sm.gauges[s.Bucket] += intValue
			default:
				floatValue, _ = strconv.ParseFloat(s.Value, 32)
				sm.counters[s.Bucket] += int(float32(floatValue) * (1 / s.Sampling))
			}
		}
	}
	log.Println("StatsdMonitor for input stopped: ", sm.ir.Name())
	wg.Done()
}

// Extracts all of the accumulated data and generates and injects a statmetric
// message into the Heka pipeline.
func (sm *statMonitor) Flush() {
	var value float64
	var intval int64
	numStats := 0
	now := time.Now().UTC()
	nowUnix := now.Unix()
	buffer := bytes.NewBufferString("")
	for s, c := range sm.counters {
		value = float64(c) / ((float64(sm.flushInterval) * float64(time.Second)) / float64(1e3))
		fmt.Fprintf(buffer, "stats.%s %f %d\n", s, value, nowUnix)
		fmt.Fprintf(buffer, "stats_counts.%s %d %d\n", s, c, nowUnix)
		sm.counters[s] = 0
		numStats++
	}
	for i, g := range sm.gauges {
		intval = int64(g)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", i, intval, nowUnix)
		numStats++
	}
	var min, max, mean, maxAtThreshold, sum float64
	var count, thresholdIndex, numInThreshold, i int
	var values []float64
	for u, t := range sm.timers {
		if len(t) > 0 {
			sort.Float64s(t)
			min = t[0]
			max = t[len(t)-1]
			mean = min
			maxAtThreshold = max
			count = len(t)
			if len(t) > 1 {
				thresholdIndex = ((100 - sm.percentThreshold) / 100) * count
				numInThreshold = count - thresholdIndex
				values = t[0:numInThreshold]

				sum = float64(0)
				for i = 0; i < numInThreshold; i++ {
					sum += values[i]
				}
				mean = sum / float64(numInThreshold)
			}
			sm.timers[u] = t[:0]

			fmt.Fprintf(buffer, "stats.timers.%s.mean %f %d\n", u, mean, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %f %d\n", u, max, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %f %d\n", u,
				sm.percentThreshold, maxAtThreshold, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %f %d\n", u, min, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, count, nowUnix)
		} else {
			// Need to still submit timers as zero
			fmt.Fprintf(buffer, "stats.timers.%s.mean %d %d\n", u, 0, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", u, 0, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %d %d\n", u,
				sm.percentThreshold, 0, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", u, 0, nowUnix)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, 0, nowUnix)
		}
		numStats++
	}
	fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, nowUnix)
	pack := <-sm.ir.InChan()
	pack.Message.SetType("statmetric")
	pack.Message.SetTimestamp(now.UnixNano())
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetHostname(sm.h.PipelineConfig().hostname)
	pack.Message.SetPid(sm.h.PipelineConfig().pid)
	pack.Message.SetPayload(buffer.String())
	sm.ir.Inject(pack)
	return
}

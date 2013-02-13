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
	"github.com/rafrombrc/go-notify"
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

// StatsInput Configuration
type StatsdInputConfig struct {
	// UDP Address to listen to for statsd packets, if left blank, no
	// UDP listener will be established
	Address string
	// How frequently to flush aggregated statsd metrics
	FlushInterval int64
	// Percent threshold to use for
	PercentThreshold int
}

// Statsd Input handles statsd metric style input and flushes aggregated
// values
//
// It can listen on a UDP address if configured to do so for standard
// statsd packets of message type Counter, Gauge, or Timer. It currently
// doesn't support Sets or other metric types.
type StatsdInput struct {
	// Channel for StatPackets, these are fed in by UDP when configured or
	// can be directly sent in from other Plugins as needed.
	Packet chan<- StatPacket

	name             string
	listener         *net.UDPConn
	percentThreshold int
	flushInterval    int64
	stopped          bool
}

// A StatPacket appropriate for a plugin to feed directly into the
// StatsdInput.Packet channel
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

	udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
	if err != nil {
		return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
	}
	s.listener, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
	}
	return nil
}

func (s *StatsdInput) SetName(name string) {
	s.name = name
}

func (s *StatsdInput) Name() string {
	return s.name
}

func (s *StatsdInput) Start(config *PipelineConfig, wg *sync.WaitGroup) error {
	packets := make(chan StatPacket, 5000)
	s.Packet = packets
	sm := new(statMonitor)
	go sm.Monitor(s, packets, wg)

	// Spin up the UDP listener if it was configured
	if s.listener != nil {
		wg.Add(1)
		go func() {
			var n int
			var err error
			defer s.listener.Close()
			timeout := time.Duration(time.Millisecond * 100)
			for {
				if s.stopped {
					break
				}
				message := make([]byte, 512)
				s.listener.SetReadDeadline(time.Now().Add(timeout))
				n, _, err = s.listener.ReadFromUDP(message)
				if err != nil || n == 0 {
					continue
				}
				go s.handleMessage(message[:n])
			}
			log.Println("StatsdUdpInput for input stopped: ", s.Name())
			wg.Done()
		}()
	}
	return nil
}

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

type statMonitor struct {
	counters         map[string]int
	timers           map[string][]float64
	gauges           map[string]int
	percentThreshold int
	flushInterval    int64
}

func (sm *statMonitor) Monitor(input *StatsdInput, packets <-chan StatPacket,
	wg *sync.WaitGroup) {
	var s StatPacket
	var floatValue float64
	var intValue int

	sm.counters = make(map[string]int)
	sm.timers = make(map[string][]float64)
	sm.gauges = make(map[string]int)
	sm.percentThreshold = input.percentThreshold
	sm.flushInterval = input.flushInterval

	stopChan := make(chan interface{})
	notify.Start(STOP, stopChan)

	t := time.NewTicker(time.Duration(input.flushInterval) * time.Second)
statLoop:
	for {
		select {
		case <-stopChan:
			sm.Flush()
			break statLoop
		case <-t.C:
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
	log.Println("StatsdMonitor for input stopped: ", input.Name())
	input.stopped = true
	wg.Done()
}

func (sm *statMonitor) Flush() {
	var value float64
	var intval int64
	numStats := 0
	now := time.Now()
	buffer := bytes.NewBufferString("")
	for s, c := range sm.counters {
		value = float64(c) / ((float64(sm.flushInterval) * float64(time.Second)) / float64(1e3))
		fmt.Fprintf(buffer, "stats.%s %d %d\n", s, value, now)
		fmt.Fprintf(buffer, "stats_counts.%s %d %d\n", s, c, now)
		sm.counters[s] = 0
		numStats++
	}
	for i, g := range sm.gauges {
		intval = int64(g)
		fmt.Fprintf(buffer, "stats.%s %d %d\n", i, intval, now)
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

			fmt.Fprintf(buffer, "stats.timers.%s.mean %d %d\n", u, mean, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", u, max, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %d %d\n", u,
				sm.percentThreshold, maxAtThreshold, now)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", u, min, now)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, count, now)
		} else {
			// Need to still submit timers as zero
			fmt.Fprintf(buffer, "stats.timers.%s.mean %f %d\n", u, 0, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper %f %d\n", u, 0, now)
			fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %f %d\n", u,
				sm.percentThreshold, 0, now)
			fmt.Fprintf(buffer, "stats.timers.%s.lower %f %d\n", u, 0, now)
			fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, 0, now)
		}
		numStats++
	}
	fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, now)
	newMsg := MessageGenerator.Retrieve(0)
	newMsg.Message.SetType("statmetric")
	newMsg.Message.SetTimestamp(now.UnixNano())
	newMsg.Message.SetPayload(buffer.String())
	MessageGenerator.Inject(newMsg)
	return
}

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
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"
)

var (
	sanitizeRegexp = regexp.MustCompile("[^a-zA-Z0-9\\-_\\.:\\|@]")
	packetRegexp   = regexp.MustCompile("([a-zA-Z0-9_\\.]+):(\\-?[0-9\\.]+)\\|(c|ms|g)(\\|@([0-9\\.]+))?")
)

// A Heka Input plugin that handles statsd metric style input and flushes
// aggregated values. It can listen on a UDP address if configured to do so
// for standard statsd packets of message type Counter, Gauge, or Timer. It
// also accepts StatPacket objects generated from within Heka itself (usually
// via a configured StatFilter plugin) over the exposed `Packet` channel. It
// currently doesn't support Sets or other metric types.
type StatsdInput struct {
	name          string
	listener      net.Conn
	stopChan      chan bool
	statChan      chan<- Stat
	statAccumName string
	statAccum     StatAccumulator
	ir            InputRunner
}

// StatsInput config struct
type StatsdInputConfig struct {
	// UDP Address to listen to for statsd packets. Defaults to
	// "127.0.0.1:8125".
	Address string
	// Configured name of StatAccumInput plugin to which this filter should be
	// delivering its stats. Defaults to "StatsAccumInput".
	StatAccumName string `toml:"stat_accum_name"`
}

func (s *StatsdInput) ConfigStruct() interface{} {
	return &StatsdInputConfig{
		Address:       "127.0.0.1:8125",
		StatAccumName: "StatAccumInput",
	}
}

func (s *StatsdInput) Init(config interface{}) error {
	conf := config.(*StatsdInputConfig)
	udpAddr, err := net.ResolveUDPAddr("udp", conf.Address)
	if err != nil {
		return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
	}
	s.listener, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
	}
	s.statAccumName = conf.StatAccumName
	return nil
}

// Spins up a statsd server listening on a UDP connection.
func (s *StatsdInput) Run(ir InputRunner, h PluginHelper) (err error) {
	s.stopChan = make(chan bool)
	s.ir = ir

	if s.statAccum, err = h.StatAccumulator(s.statAccumName); err != nil {
		return
	}

	// Spin up the UDP listener.
	var (
		n       int
		e       error
		stopped bool
	)
	defer s.listener.Close()
	timeout := time.Duration(time.Millisecond * 100)

	for !stopped {
		message := make([]byte, 512)
		s.listener.SetReadDeadline(time.Now().Add(timeout))
		n, e = s.listener.Read(message)

		select {
		case <-s.stopChan:
			stopped = true
		default:
		}

		if e != nil || n == 0 {
			continue
		}

		if stopped {
			// If we're stopping, use synchronous call so we don't
			// close the Packet channel too soon.
			s.handleMessage(message[:n])
		} else {
			go s.handleMessage(message[:n])
		}
	}

	return
}

func (s *StatsdInput) Stop() {
	close(s.stopChan)
}

// Parses received raw statsd bytes data and converts it into a StatPacket
// object that can be passed to the StatMonitor.
func (s *StatsdInput) handleMessage(message []byte) {
	var (
		value string
		stat  Stat
	)
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

		stat.Bucket = item[1]
		stat.Value = value
		stat.Modifier = item[3]
		stat.Sampling = float32(sampleRate)
		if !s.statAccum.DropStat(stat) {
			s.ir.LogError(fmt.Errorf("Undelivered stat: %s", stat))
		}
	}
}

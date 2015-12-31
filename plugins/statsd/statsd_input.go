/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package statsd

import (
	"bytes"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"math"
	"net"
	"strconv"
	"time"
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
	maxMsgSize    uint
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
	// Size of a message read from statsd. In some cases, when statsd
	// sends a lots in single message of stats it's required to boost this value.
	// Defaults to 512.
	MaxMsgSize uint `toml:"max_msg_size"`
}

func (s *StatsdInput) ConfigStruct() interface{} {
	return &StatsdInputConfig{
		Address:       "127.0.0.1:8125",
		StatAccumName: "StatAccumInput",
		MaxMsgSize:    512,
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
	s.maxMsgSize = conf.MaxMsgSize
	s.stopChan = make(chan bool)
	return nil
}

// Spins up a statsd server listening on a UDP connection.
func (s *StatsdInput) Run(ir InputRunner, h PluginHelper) (err error) {
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
		message := make([]byte, s.maxMsgSize)
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

		s.handleMessage(message[:n])
	}

	return
}

func (s *StatsdInput) Stop() {
	close(s.stopChan)
}

// Parses received raw statsd bytes data and converts it into a StatPacket
// object that can be passed to the StatMonitor.
func (s *StatsdInput) handleMessage(message []byte) {
	stats, badLines := parseMessage(message)
	for _, line := range badLines {
		s.ir.LogError(fmt.Errorf("can't parse message: %s", string(line)))
	}
	for _, stat := range stats {
		if !s.statAccum.DropStat(stat) {
			s.ir.LogError(fmt.Errorf("undelivered stat: %+v", stat))
		}
	}
}

func parseMessage(message []byte) ([]Stat, [][]byte) {
	message = bytes.Trim(message, " \t\n")

	stats := make([]Stat, 0, int(math.Max(1, float64(bytes.Count(message, []byte("\n"))))))

	var lines [][]byte
	if bytes.IndexByte(message, '\n') > -1 {
		lines = bytes.Split(message, []byte("\n"))
	} else {
		lines = [][]byte{message}
	}

	badLines := make([][]byte, 0, 2)

	for _, s_line := range lines {
		//trim white space
		line := bytes.Trim(s_line, " \t\n")

		// skip blank lines
		if len(line) == 0 {
			continue
		}

		colonPos := bytes.IndexByte(line, ':')
		if colonPos == -1 || len(line) < colonPos+3 {
			badLines = append(badLines, line)
			continue
		}

		pipePos := bytes.IndexByte(line[colonPos+1:], '|') + colonPos + 1
		if pipePos == -1 || pipePos == colonPos || len(line) < pipePos+2 {
			badLines = append(badLines, line)
			continue
		}

		bucket := line[:colonPos]
		value := line[colonPos+1 : pipePos]
		modifier, sampleMaybe, err := extractModifier(line[pipePos+1:])
		if err != nil {
			badLines = append(badLines, line)
			continue
		}

		sampleRate := float32(1)
		if sampleMaybe {
			sampleRate, err = extractSampleRate(line[pipePos+2:])
			if err != nil {
				badLines = append(badLines, line)
				continue
			}
		}

		var stat Stat
		stat.Bucket = string(bucket)
		stat.Value = string(value)
		stat.Modifier = string(modifier)
		stat.Sampling = sampleRate

		stats = append(stats, stat)
	}

	return stats, badLines
}

func extractModifier(message []byte) ([]byte, bool, error) {
	l := len(message)
	switch {
	case l == 1:
		for _, m := range []byte{'g', 'h', 'm', 'c'} {
			if message[0] == m {
				return message, false, nil
			}
		}
	case l == 2:
		if bytes.Equal(message, []byte("ms")) {
			return []byte("ms"), false, nil
		}
	case l > 2:
		if message[0] == 'c' {
			return []byte("c"), true, nil
		}
		if message[0] == 'g' {
			return []byte("g"), true, nil
		}
		if bytes.HasPrefix(message, []byte("ms")) {
			return []byte("ms"), true, nil
		}
	}
	return []byte{}, false, fmt.Errorf("invalid modifier in message %s", message)
}

func extractSampleRate(message []byte) (float32, error) {
	l := len(message)
	if l > 0 && message[0] == 's' {
		// Leftover "s" from "ms" modifier.
		l = l - 1
		message = message[1:]
	}
	if l < 3 || !bytes.HasPrefix(message, []byte("|@")) {
		return 1, nil
	}
	sampleRate, err := strconv.ParseFloat(string(message[2:]), 32)
	if err != nil {
		return 1, err
	}
	return float32(sampleRate), nil
}

func init() {
	RegisterPlugin("StatsdInput", func() interface{} {
		return new(StatsdInput)
	})
}

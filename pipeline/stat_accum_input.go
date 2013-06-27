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
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"log"
	"sort"
	"strconv"
	"time"
)

// Represents a single stat value in the format expected by the StatAccumInput.
type Stat struct {
	Bucket   string
	Value    string
	Modifier string
	Sampling float32
}

// Specialized Input that listens on a provided channel for Stat objects, from
// which it accumulates and stores statsd-style metrics data, periodically
// generating and injecting messages with accumulated stats date embedded in
// either the payload, the message fields, or both.
type StatAccumulator interface {
	DropStat(stat Stat) (sent bool)
}

type StatAccumInput struct {
	statChan chan Stat
	counters map[string]int
	timers   map[string][]float64
	gauges   map[string]int
	pConfig  *PipelineConfig
	config   *StatAccumInputConfig
	ir       InputRunner
	tickChan <-chan time.Time
	stopChan chan bool
}

type StatAccumInputConfig struct {
	EmitInPayload    bool   `toml:"emit_in_payload"`
	EmitInFields     bool   `toml:"emit_in_fields"`
	PercentThreshold int    `toml:"percent_threshold"`
	FlushInterval    int64  `toml:"flush_interval"`
	MessageType      string `toml:"message_type"`
	TickerInterval   uint   `toml:"ticker_interval"`
}

func (sm *StatAccumInput) ConfigStruct() interface{} {
	return &StatAccumInputConfig{
		EmitInFields:     true,
		PercentThreshold: 90,
		FlushInterval:    10,
		MessageType:      "heka.statmetric",
		TickerInterval:   uint(10),
	}
}

func (sm *StatAccumInput) Init(config interface{}) error {
	sm.counters = make(map[string]int)
	sm.timers = make(map[string][]float64)
	sm.gauges = make(map[string]int)
	sm.statChan = make(chan Stat, Globals().PoolSize)
	sm.stopChan = make(chan bool, 1)

	sm.config = config.(*StatAccumInputConfig)
	if !sm.config.EmitInPayload && !sm.config.EmitInFields {
		return errors.New(
			"One of either `EmitInPayload` or `EmitInFields` must be set to true.",
		)
	}
	return nil
}

// Listens on the Stat channel for stats generated internally by Heka.
func (sm *StatAccumInput) Run(ir InputRunner, h PluginHelper) (err error) {
	var (
		stat       Stat
		floatValue float64
		intValue   int
	)

	sm.pConfig = h.PipelineConfig()
	sm.ir = ir
	sm.tickChan = sm.ir.Ticker()
	ok := true
	for ok {
		select {
		case <-sm.tickChan:
			sm.Flush()
		case stat, ok = <-sm.statChan:
			if !ok {
				sm.Flush()
				break
			}
			switch stat.Modifier {
			case "ms":
				floatValue, _ = strconv.ParseFloat(stat.Value, 64)
				sm.timers[stat.Bucket] = append(sm.timers[stat.Bucket], floatValue)
			case "g":
				intValue, _ = strconv.Atoi(stat.Value)
				sm.gauges[stat.Bucket] = intValue
			default:
				floatValue, _ = strconv.ParseFloat(stat.Value, 32)
				sm.counters[stat.Bucket] += int(float32(floatValue) * (1 / stat.Sampling))
			}
		}
	}

	return
}

func (sm *StatAccumInput) Stop() {
	// Closing the stopChan first so DropStat won't put any stats on
	// the statChan after it's closed.
	close(sm.stopChan)
	close(sm.statChan)
}

// Drops a Stat pointer onto the input's stat channel for processing. Returns
// true if the stat was processed, false if the input was already closing and
// couldn't process the stat.
func (sm *StatAccumInput) DropStat(stat Stat) (sent bool) {
	select {
	case <-sm.stopChan:
	default:
		sm.statChan <- stat
		sent = true
	}
	return
}

// Extracts all of the accumulated data and generates and injects a message
// into the Heka pipeline.
func (sm *StatAccumInput) Flush() {
	var (
		value         float64
		intval        int64
		field         *message.Field
		err           error
		sName, scName string
	)
	numStats := 0
	now := time.Now().UTC()
	nowUnix := now.Unix()
	buffer := bytes.NewBufferString("")
	pack := <-sm.ir.InChan()

	newField := func(name string, value interface{}) {
		if field, err = message.NewField(name, value, ""); err == nil {
			pack.Message.AddField(field)
		} else {
			log.Println("StatAccumInput can't add field: ", name)
		}
	}

	if sm.config.EmitInFields {
		newField("timestamp", nowUnix)
	}

	for s, c := range sm.counters {
		value = float64(c) / ((float64(sm.config.FlushInterval) * float64(time.Second)) / float64(1e3))
		sName = fmt.Sprintf("stats.%s", s)
		scName = fmt.Sprintf("stats_counts.%s", s)
		if sm.config.EmitInPayload {
			fmt.Fprintf(buffer, "%s %f %d\n", sName, value, nowUnix)
			fmt.Fprintf(buffer, "%s %d %d\n", scName, c, nowUnix)
		}
		if sm.config.EmitInFields {
			newField(sName, int(value))
			newField(scName, c)
		}
		sm.counters[s] = 0
		numStats++
	}
	for i, g := range sm.gauges {
		intval = int64(g)
		sName = fmt.Sprintf("stats.%s", i)
		if sm.config.EmitInPayload {
			fmt.Fprintf(buffer, "%s %d %d\n", sName, intval, nowUnix)
		}
		if sm.config.EmitInFields {
			newField(sName, intval)
		}
		numStats++
	}

	var (
		min, max, mean, maxAtThreshold, sum      float64
		count, thresholdIndex, numInThreshold, i int
		values                                   []float64
	)

	for u, t := range sm.timers {
		if len(t) > 0 {
			sort.Float64s(t)
			min = t[0]
			max = t[len(t)-1]
			mean = min
			maxAtThreshold = max
			count = len(t)
			if len(t) > 1 {
				thresholdIndex = ((100 - sm.config.PercentThreshold) / 100) * count
				numInThreshold = count - thresholdIndex
				values = t[0:numInThreshold]

				sum = float64(0)
				for i = 0; i < numInThreshold; i++ {
					sum += values[i]
				}
				mean = sum / float64(numInThreshold)
			}
			sm.timers[u] = t[:0]

			if sm.config.EmitInPayload {
				fmt.Fprintf(buffer, "stats.timers.%s.mean %f %d\n", u, mean, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.upper %f %d\n", u, max, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %f %d\n", u,
					sm.config.PercentThreshold, maxAtThreshold, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.lower %f %d\n", u, min, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, count, nowUnix)
			}

			if sm.config.EmitInFields {
				newField(fmt.Sprintf("stats.timers.%s.mean", u), mean)
				newField(fmt.Sprintf("stats.timers.%s.upper", u), max)
				newField(fmt.Sprintf("stats.timers.%s.upper_%d", u, sm.config.PercentThreshold),
					maxAtThreshold)
				newField(fmt.Sprintf("stats.timers.%s.lower", u), min)
				newField(fmt.Sprintf("stats.timers.%s.count", u), count)
			}
		} else {
			if sm.config.EmitInPayload {
				// Need to still submit timers as zero
				fmt.Fprintf(buffer, "stats.timers.%s.mean %d %d\n", u, 0, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", u, 0, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.upper_%d %d %d\n", u,
					sm.config.PercentThreshold, 0, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", u, 0, nowUnix)
				fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", u, 0, nowUnix)
			}
			if sm.config.EmitInFields {
				newField(fmt.Sprintf("stats.timers.%s.mean", u), 0)
				newField(fmt.Sprintf("stats.timers.%s.upper", u), 0)
				newField(fmt.Sprintf("stats.timers.%s.upper_%d", u, sm.config.PercentThreshold),
					0)
				newField(fmt.Sprintf("stats.timers.%s.lower", u), 0)
				newField(fmt.Sprintf("stats.timers.%s.count", u), 0)
			}
		}
		numStats++
	}
	if sm.config.EmitInPayload {
		fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, nowUnix)
	}
	if sm.config.EmitInFields {
		newField("statsd.numStats", numStats)
	}

	pack.Message.SetType(sm.config.MessageType)
	pack.Message.SetTimestamp(now.UnixNano())
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetHostname(sm.pConfig.hostname)
	pack.Message.SetPid(sm.pConfig.pid)
	pack.Message.SetPayload(buffer.String())
	sm.ir.Inject(pack)
}

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
	"sync"
	"time"
)

// Represents a single stat value in the format expected by the StatMonitor.
type Stat struct {
	Bucket   string
	Value    string
	Modifier string
	Sampling float32
}

// Specialized object that listens on a provided channel for StatPacket
// objects, from which it accumulates and stores statsd-style metrics data,
// periodically generating and injecting `statmetric` messages with a payload
// containing the accumulated data formatted as graphite would expect.
type StatMonitor interface {
	StatChan() chan<- Stat
}

type statMonitor struct {
	statChan chan Stat
	counters map[string]int
	timers   map[string][]float64
	gauges   map[string]int
	pConfig  *PipelineConfig
	config   *StatMonitorConfig
}

type StatMonitorConfig struct {
	EmitInPayload    bool
	EmitInFields     bool
	PercentThreshold int
	FlushInterval    int64
}

func (sm *statMonitor) ConfigStruct() interface{} {
	return &StatMonitorConfig{
		EmitInFields:     true,
		PercentThreshold: 90,
		FlushInterval:    10,
	}
}

// Returns a new statMonitor object.
func newStatMonitor(pConfig *PipelineConfig) *statMonitor {
	return &statMonitor{
		counters: make(map[string]int),
		timers:   make(map[string][]float64),
		gauges:   make(map[string]int),
		pConfig:  pConfig,
	}
}

func (sm *statMonitor) Init(config interface{}) error {
	sm.config = config.(*StatMonitorConfig)
	if !sm.config.EmitInPayload && !sm.config.EmitInFields {
		return errors.New(
			"One of either `EmitInPayload` or `EmitInFields` must be set to true.",
		)
	}
	return nil
}

func (sm *statMonitor) StatChan() chan<- Stat {
	return sm.statChan
}

// Main statMonitor loop. Doesn't return until statChan is closed.
func (sm *statMonitor) Monitor(wg *sync.WaitGroup) {
	var stat Stat
	var floatValue float64
	var intValue int

	t := time.Tick(time.Duration(sm.config.FlushInterval) * time.Second)
	ok := true
	for ok {
		select {
		case <-t:
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
	log.Println("StatMonitor stopped")
	wg.Done()
}

// Extracts all of the accumulated data and generates and injects a statmetric
// message into the Heka pipeline.
func (sm *statMonitor) Flush() {
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
	pack := sm.pConfig.PipelinePack(0)

	newField := func(name string, value interface{}) {
		if field, err = message.NewField(name, value, ""); err == nil {
			pack.Message.AddField(field)
		} else {
			log.Println("StatMonitor can't add field: ", name)
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
	fmt.Fprintf(buffer, "statsd.numStats %d %d\n", numStats, nowUnix)
	pack.Message.SetType("statmetric")
	pack.Message.SetTimestamp(now.UnixNano())
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetHostname(sm.pConfig.hostname)
	pack.Message.SetPid(sm.pConfig.pid)
	pack.Message.SetPayload(buffer.String())
	sm.pConfig.router.inChan <- pack
	return
}

/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
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
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
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
	gauges   map[string]float64
	pConfig  *PipelineConfig
	config   *StatAccumInputConfig
	ir       InputRunner
	tickChan <-chan time.Time
	inChan   chan *PipelinePack
	stopChan chan bool
}

type StatAccumInputConfig struct {
	// Specifies whether or not stats data should be written to outgoing
	// message payloads, in the string format that is expected by a listening
	// Graphite Carbon server. Defaults to true.
	EmitInPayload bool `toml:"emit_in_payload"`

	// Specifies whether or not stats data should be written to outgoing
	// message fields. Defaults to false. At least one of `EmitInPayload` or
	// `EmitInFields` *must* be true or there will be a config error.
	EmitInFields bool `toml:"emit_in_fields"`

	// Percentage threshold to use for calculating "upper N%" type numerical
	// statistics. Defaults to 90.
	PercentThreshold []int `toml:"percent_threshold"`

	// Type value to use for outgoing stat messages, defaults to
	// `heka.statmetric`.
	MessageType string `toml:"message_type"`

	// Interval at which the generated stat messages should be emitted, in
	// seconds. Defaults to 10.
	TickerInterval uint `toml:"ticker_interval"`

	// The remaining settings are used to specify the namespaces used for
	// various stat types, modeled on the behavior of etsy's standalond statsd
	// server implementation, see
	// https://github.com/etsy/statsd/blob/master/docs/namespacing.md.
	LegacyNamespaces bool   `toml:"legacy_namespaces"`
	GlobalPrefix     string `toml:"global_prefix"`
	CounterPrefix    string `toml:"counter_prefix"`
	TimerPrefix      string `toml:"timer_prefix"`
	GaugePrefix      string `toml:"gauge_prefix"`
	StatsdPrefix     string `toml:"statsd_prefix"`

	// Don't emit values for inactive stats instead of sending 0 or in the case
	// of gauges, sending the previous value. Defaults to false
	DeleteIdleStats bool `toml:"delete_idle_stats"`
}

func (sm *StatAccumInput) ConfigStruct() interface{} {
	return &StatAccumInputConfig{
		EmitInPayload:    true,
		PercentThreshold: []int{90},
		MessageType:      "heka.statmetric",
		TickerInterval:   uint(10),
		LegacyNamespaces: false,
		StatsdPrefix:     "statsd",
		GlobalPrefix:     "stats",
		CounterPrefix:    "counters",
		TimerPrefix:      "timers",
		GaugePrefix:      "gauges",
		DeleteIdleStats:  false,
	}
}

// Heka will call this before calling any other methods to give us access to
// the pipeline configuration.
func (sm *StatAccumInput) SetPipelineConfig(pConfig *PipelineConfig) {
	sm.pConfig = pConfig
}

func (sm *StatAccumInput) Init(config interface{}) error {
	sm.counters = make(map[string]int)
	sm.timers = make(map[string][]float64)
	sm.gauges = make(map[string]float64)
	sm.statChan = make(chan Stat, sm.pConfig.Globals.PoolSize)
	sm.stopChan = make(chan bool, 1)

	sm.config = config.(*StatAccumInputConfig)
	if sm.config.TickerInterval == 0 {
		return errors.New(
			"TickerInterval must be greater than 0.",
		)
	}
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
	)

	sm.ir = ir
	sm.inChan = ir.InChan()
	sm.tickChan = ir.Ticker()
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
				floatValue, _ = strconv.ParseFloat(stat.Value, 64)
				sm.gauges[stat.Bucket] = floatValue
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
		field *message.Field
		err   error
	)
	numStats := 0
	now := time.Now().UTC()
	nowUnix := now.Unix()
	buffer := bytes.NewBufferString("")
	pack := <-sm.inChan

	rootNs := NewRootNamespace()

	if sm.config.EmitInFields {
		rootNs.Emitters.EmitInField = func(name string, value interface{}) {
			if field, err = message.NewField(name, value, ""); err == nil {
				pack.Message.AddField(field)
			} else {
				sm.ir.LogError(fmt.Errorf("can't add field: %s", name))
			}
		}
	}

	if sm.config.EmitInPayload {
		rootNs.Emitters.EmitInPayload = func(name string, value interface{}) {
			switch value.(type) {
			default:
				fmt.Fprintf(buffer, "%s %v %d\n", name, value, nowUnix)
			case float64:
				fmt.Fprintf(buffer, "%s %f %d\n", name, value, nowUnix)
			}
		}
	}
	rootNs.EmitInField("timestamp", nowUnix)

	globalNs := rootNs.Namespace(sm.config.GlobalPrefix)
	counterNs := globalNs.Namespace(sm.config.CounterPrefix)
	for key, c := range sm.counters {
		ratePerSecond := float64(c) / float64(sm.config.TickerInterval)
		if sm.config.LegacyNamespaces {
			globalNs.EmitInField(key, int(ratePerSecond))
			globalNs.EmitInPayload(key, ratePerSecond)
			rootNs.Namespace("stats_counts").Emit(key, c)
		} else {
			counterKey := counterNs.Namespace(key)
			counterKey.Emit("rate", ratePerSecond)
			counterKey.Emit("count", c)
		}
		if sm.config.DeleteIdleStats {
			delete(sm.counters, key)
		} else {
			sm.counters[key] = 0
		}
		numStats++
	}
	for key, gauge := range sm.gauges {
		globalNs.Namespace(sm.config.GaugePrefix).Emit(key, float64(gauge))
		if sm.config.DeleteIdleStats {
			delete(sm.gauges, key)
		}
		numStats++
	}
	countPercentThreshold := len(sm.config.PercentThreshold)
	for key, timings := range sm.timers {
		timerNs := globalNs.Namespace(sm.config.TimerPrefix).Namespace(key)
		var min, max, sum, mean, rate, thresholdBoundary  float64
		meanPercentile := make([]float64, countPercentThreshold)
		upperPercentile := make([]float64, countPercentThreshold)
		count := len(timings)
		if count > 0 {
			sort.Float64s(timings)

			cumulativeValues := make([]float64, count)
			cumulativeValues[0] = timings[0]
			for i := 1; i < count; i++ {
				cumulativeValues[i] = timings[i] + cumulativeValues[i-1]
			}

			rate = float64(count) / float64(sm.config.TickerInterval)
			min = timings[0]
			max = timings[count-1]
			mean = min

			for i := 0; i < countPercentThreshold; i++{
				tmp := ((100.0 - float64(sm.config.PercentThreshold[i])) / 100.0) * float64(count)
				numInThreshold := count - int(math.Floor(tmp+0.5)) // simulate JS Math.round(x)

				if numInThreshold > 0 {
					mean = cumulativeValues[numInThreshold-1] / float64(numInThreshold)
					thresholdBoundary = timings[numInThreshold-1]
				} else {
					mean = min
					thresholdBoundary = max
				}
				meanPercentile[i] = mean
				upperPercentile[i] = thresholdBoundary
			}

			sum = cumulativeValues[len(cumulativeValues)-1]
			mean = sum / float64(count)
		} else {
			rate = 0.
			min = 0.
			max = 0.
			sum = 0.
			mean = 0.
		}

		timerNs.Emit("count", count)
		timerNs.Emit("count_ps", rate)
		timerNs.Emit("lower", min)
		timerNs.Emit("upper", max)
		timerNs.Emit("sum", sum)
		timerNs.Emit("mean", mean)
		for i := 0; i < countPercentThreshold; i++ {
			timerNs.Emit(fmt.Sprintf("mean_%d", sm.config.PercentThreshold[i]), meanPercentile[i])
			timerNs.Emit(fmt.Sprintf("upper_%d", sm.config.PercentThreshold[i]), upperPercentile[i])
		}

		if sm.config.DeleteIdleStats {
			delete(sm.timers, key)
		} else {
			sm.timers[key] = timings[:0]
		}
		numStats++
	}

	if sm.config.LegacyNamespaces {
		rootNs.Namespace(sm.config.StatsdPrefix).Emit("numStats", numStats)
	} else {
		globalNs.Namespace(sm.config.StatsdPrefix).Emit("numStats", numStats)
	}

	pack.Message.SetLogger(sm.ir.Name())
	pack.Message.SetType(sm.config.MessageType)
	pack.Message.SetTimestamp(now.UnixNano())
	pack.Message.SetUuid(uuid.NewRandom())
	pack.Message.SetHostname(sm.pConfig.hostname)
	pack.Message.SetPid(sm.pConfig.pid)
	pack.Message.SetPayload(buffer.String())
	sm.ir.Inject(pack)
}

type statsEmitters struct {
	EmitInPayload func(key string, value interface{})
	EmitInField   func(key string, value interface{})
}

type namespaceTree struct {
	prefix   string
	Emitters *statsEmitters
	parent   *namespaceTree
}

func NewRootNamespace() *namespaceTree {
	ns := new(namespaceTree)
	ns.Emitters = new(statsEmitters)
	return ns
}
func (ns *namespaceTree) setNamespace(namespace string) {
	var prefix string
	if ns.parent != nil {
		prefix = ns.parent.prefix
	}
	if len(namespace) == 0 || namespace[len(namespace)-1] == '.' {
		ns.prefix = prefix + namespace
	} else {
		ns.prefix = prefix + namespace + "."
	}
}

func (ns *namespaceTree) Namespace(namespace string) *namespaceTree {
	n := namespaceTree{"", ns.Emitters, ns}
	n.setNamespace(namespace)
	return &n
}

func (ns *namespaceTree) EmitInField(key string, value interface{}) *namespaceTree {
	if ns.Emitters.EmitInField != nil {
		ns.Emitters.EmitInField(ns.prefix+key, value)
	}
	return ns
}

func (ns *namespaceTree) EmitInPayload(key string, value interface{}) *namespaceTree {
	if ns.Emitters.EmitInPayload != nil {
		ns.Emitters.EmitInPayload(ns.prefix+key, value)
	}
	return ns
}

func (ns *namespaceTree) Emit(key string, value interface{}) *namespaceTree {
	if ns.Emitters.EmitInPayload != nil {
		ns.Emitters.EmitInPayload(ns.prefix+key, value)
	}
	if ns.Emitters.EmitInField != nil {
		ns.Emitters.EmitInField(ns.prefix+key, value)
	}
	return ns
}

func init() {
	RegisterPlugin("StatAccumInput", func() interface{} {
		return new(StatAccumInput)
	})
}

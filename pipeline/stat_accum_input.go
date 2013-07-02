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
	"math"
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
	// Specifies whether or not stats data should be written to outgoing
	// message payloads, in the string format that is expected by a listening
	// Graphite Carbon server. Defaults to false.
	EmitInPayload bool `toml:"emit_in_payload"`

	// Specifies whether or not stats data should be written to outgoing
	// message fields. Defaults to true. At least one of `EmitInPayload` or
	// `EmitInFields` *must* be true or there will be a config error.
	EmitInFields bool `toml:"emit_in_fields"`

	// Percentage threshold to use for calculating "upper N%" type numerical
	// statistics. Defaults to 90.
	PercentThreshold int `toml:"percent_threshold"`

	// Type value to use for outgoing stat messages, defaults to
	// `heka.statmetric`.
	MessageType string `toml:"message_type"`

	// Interval at which the generated stat messages should be emitted, in
	// seconds. Defaults to 10.
	TickerInterval uint `toml:"ticker_interval"`
}

func (sm *StatAccumInput) ConfigStruct() interface{} {
	return &StatAccumInputConfig{
		EmitInFields:     true,
		PercentThreshold: 90,
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
		value float64
		field *message.Field
		err   error
	)
	numStats := 0
	now := time.Now().UTC()
	nowUnix := now.Unix()
	buffer := bytes.NewBufferString("")
	pack := <-sm.ir.InChan()

	rootNs := NewRootNamespace("")

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
	statsNs := rootNs.Namespace("stats")

	for key, c := range sm.counters {
		value = float64(c) / ((float64(sm.config.TickerInterval) *
			float64(time.Second)) / float64(1e3))
		statsNs.EmitInField(key, int(value))
		statsNs.EmitInPayload(key, value)
		rootNs.Namespace("stats_counts").Emit(key, c)
		sm.counters[key] = 0
		numStats++
	}
	for key, g := range sm.gauges {
		rootNs.Namespace("stats").Emit(key, int64(g))
		numStats++
	}

	for u, t := range sm.timers {
		if len(t) > 0 {
			sort.Float64s(t)
			min := t[0]
			max := t[len(t)-1]
			timerNs := statsNs.Namespace("timers").Namespace(u)
			count := len(t)
			if count > 1 {
				tmp := ((100.0 - float64(sm.config.PercentThreshold)) / 100.0) * float64(count)
				numInThreshold := count - int(math.Floor(tmp+0.5)) // simulate JS Math.round(x)
				values := t[0:numInThreshold]
				max := t[numInThreshold-1]
				var sum float64
				for _, v := range values {
					sum += v
				}
				mean := sum / float64(numInThreshold)
				timerNs.Emit(fmt.Sprintf("upper_%d", sm.config.PercentThreshold), max)
				timerNs.Emit(fmt.Sprintf("mean_%d", sm.config.PercentThreshold), mean)
			}

			sm.timers[u] = t[:0]
			var sum float64
			for _, v := range t {
				sum += v
			}
			mean := sum / float64(count)

			timerNs.Emit("mean", mean)
			timerNs.Emit("upper", max)
			timerNs.Emit("lower", min)
			timerNs.Emit("count", count)
			numStats++
		}
	}
	rootNs.Namespace("statsd").Emit("numStats", numStats)

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

func NewRootNamespace(namespace string) *namespaceTree {
	ns := new(namespaceTree)
	ns.setNamespace(namespace)
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

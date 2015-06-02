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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package statsd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
)

// Simple struct representing a single statsd-style metric value.
type metric struct {
	// Supports "Counter", "Timer", or "Gauge"
	Type_      string `toml:"type"`
	Name       string
	Value      string
	ReplaceDot bool `toml:"replace_dot"`
}

// Heka Filter plugin that can accept specific message types, extract data
// from those messages, and from that data generate statsd messages in a
// StatsdInput exactly as if a statsd message has come from a networked statsd
// client.
type StatFilter struct {
	metrics       map[string]metric
	statAccumName string
}

// StatFilter config struct.
type StatFilterConfig struct {
	// Set of metric templates this filter should use, keyed by arbitrary
	// metric id.
	Metric map[string]metric
	// Configured name of StatAccumInput plugin to which this filter should be
	// delivering its stats. Defaults to "StatsAccumInput".
	StatAccumName string `toml:"stat_accum_name"`
}

func (s *StatFilter) ConfigStruct() interface{} {
	return &StatFilterConfig{
		StatAccumName: "StatAccumInput",
	}
}

func (s *StatFilter) Init(config interface{}) (err error) {
	conf := config.(*StatFilterConfig)
	s.metrics = conf.Metric
	s.statAccumName = conf.StatAccumName
	return
}

// For each message, we first extract any match group captures, and then we
// add our own values for "Logger", "Hostname", "Type", and "Payload" as if
// they were captured values. We then iterate through all of this plugin's
// defined metrics, and for each one we use the captures to do string
// substitution on both the name and the payload. For example, a metric with
// the name "@Hostname.404s" would become a stat with the "@Hostname" replaced
// by the hostname from the received message.
func (s *StatFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	var statAccum StatAccumulator
	if statAccum, err = h.StatAccumulator(s.statAccumName); err != nil {
		return
	}

	var (
		pack   *PipelinePack
		values = make(map[string]string)
		stat   Stat
		val    string
	)

	inChan := fr.InChan()
	for pack = range inChan {
		// Load existing values into the set for replacement
		values["Logger"] = pack.Message.GetLogger()
		values["Hostname"] = pack.Message.GetHostname()
		values["Type"] = pack.Message.GetType()
		values["Payload"] = pack.Message.GetPayload()

		for _, field := range pack.Message.Fields {
			// It's painful to be converting these numeric values to strings,
			// but for now it's the only way to get numeric data into the stat
			// accumulator.
			if field.GetValueType() == message.Field_STRING && len(field.ValueString) > 0 {
				val = field.ValueString[0]
			} else if field.GetValueType() == message.Field_DOUBLE {
				val = strconv.FormatFloat(field.ValueDouble[0], 'f', -1, 64)
			} else if field.GetValueType() == message.Field_INTEGER {
				val = strconv.FormatInt(field.ValueInteger[0], 10)
			}
			values[field.GetName()] = val
		}

		// We matched, generate appropriate metrics
		for _, met := range s.metrics {
			if met.ReplaceDot {
				for key, value := range values {
					values[key] = strings.Replace(value, ".", "_", -1)
				}
			}

			stat.Bucket = InterpolateString(met.Name, values)

			switch met.Type_ {
			case "Counter":
				stat.Modifier = ""
			case "Timer":
				stat.Modifier = "ms"
			case "Gauge":
				stat.Modifier = "g"
			}
			stat.Value = InterpolateString(met.Value, values)
			stat.Sampling = 1.0
			if !statAccum.DropStat(stat) {
				fr.LogError(fmt.Errorf("Undelivered stat: %v", stat))
			}
		}
		fr.UpdateCursor(pack.QueueCursor)
		pack.Recycle(nil)
	}

	return
}

func init() {
	RegisterPlugin("StatFilter", func() interface{} {
		return new(StatFilter)
	})
}

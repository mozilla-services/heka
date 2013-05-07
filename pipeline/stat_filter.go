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
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
)

// Simple struct representing a single statsd-style metric value.
type metric struct {
	// Supports "Counter", "Timer", or "Gauge"
	Type_ string `toml:"type"`
	Name  string
	Value string
}

// Heka Filter plugin that can accept specific message types, extract data
// from those messages, and from that data generate statsd messages in a
// StatsdInput exactly as if a statsd message has come from a networked statsd
// client.
type StatFilter struct {
	metrics   map[string]metric
	inputName string
}

// StatFilter config struct.
type StatFilterConfig struct {
	// Set of metric templates this filter should use, keyed by arbitrary
	// metric id.
	Metric map[string]metric
	// Configured name of StatsdInput plugin to which this filter should
	// be delivering its output. Defaults to "StatsdInput".
	StatsdInputName string
}

func (s *StatFilter) ConfigStruct() interface{} {
	return &StatFilterConfig{
		StatsdInputName: "StatsdInput",
	}
}

func (s *StatFilter) Init(config interface{}) (err error) {
	conf := config.(*StatFilterConfig)
	s.metrics = conf.Metric
	s.inputName = conf.StatsdInputName
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
	inChan := fr.InChan()

	var (
		pack     *PipelinePack
		sp       StatPacket
		captures map[string]string
		ir       InputRunner
		ok       bool
	)

	// Pull the statsd input out
	ir, ok = h.PipelineConfig().InputRunners[s.inputName]
	if !ok {
		return fmt.Errorf("Unable to locate StatsdInput '%s', was it configured?",
			s.inputName)
	}
	statInput, ok := ir.Plugin().(*StatsdInput)
	if !ok {
		return fmt.Errorf("Unable to coerce '%s' input plugin to StatsdInput",
			s.inputName)
	}

	for plc := range inChan {
		pack = plc.Pack
		captures = plc.Captures
		if captures == nil {
			captures = make(map[string]string)
		}

		// Load existing fields into the set for replacement
		captures["Logger"] = pack.Message.GetLogger()
		captures["Hostname"] = pack.Message.GetHostname()
		captures["Type"] = pack.Message.GetType()
		captures["Payload"] = pack.Message.GetPayload()

		// We matched, generate appropriate metrics
		for _, met := range s.metrics {
			sp.Bucket = InterpolateString(met.Name, captures)
			switch met.Type_ {
			case "Counter":
				sp.Modifier = ""
			case "Timer":
				sp.Modifier = "ms"
			case "Gauge":
				sp.Modifier = "g"
			}
			sp.Value = InterpolateString(met.Value, captures)
			statInput.Packet <- sp
		}
		pack.Recycle()
	}

	return
}

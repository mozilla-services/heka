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
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
)

type metric struct {
	Type_ string `toml:"type"`
	Name  string
	Value string
}

type StatFilterConfig struct {
	Metric          map[string]metric
	StatsdInputName string
}

type StatFilter struct {
	metrics   map[string]metric
	inputName string
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

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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"strings"
	"sync/atomic"
)

// Interface for Heka plugins that will provide reporting data. Plugins can
// populate the Message Fields w/ arbitrary output data.
type ReportingPlugin interface {
	// When generating a Heka "self-report", Heka will check each running
	// plugin to see if it implements the `ReportingPlugin` interface. If so,
	// Heka will call the `ReportMsg` method on the plugin, passing in a
	// message struct. The plugin can populate the message fields with any
	// arbitrary information regarding the plugin's operational state that
	// might be useful in a report.
	ReportMsg(msg *message.Message) (err error)
}

// Convenience function for creating a new integer field on a message object.
func newIntField(msg *message.Message, name string, val int, representation string) {
	if f, err := message.NewField(name, val, representation); err == nil {
		msg.AddField(f)
	} else {
		fmt.Println("Report error adding int field: ", err)
	}
}

// Convenience function for creating a new int64 field on a message object.
func newInt64Field(msg *message.Message, name string, val int64, representation string) {
	if f, err := message.NewField(name, val, representation); err == nil {
		msg.AddField(f)
	} else {
		fmt.Println("Report error adding int64 field: ", err)
	}
}

// Convenience function for creating and setting a string field called "name"
// on a message object.
func newStringField(msg *message.Message, name string, val string) {
	if f, err := message.NewField(name, val, ""); err == nil {
		msg.AddField(f)
	} else {
		fmt.Println("Report error adding string field: ", err)
	}
}

// Given a PluginRunner and a Message struct, this function will populate the
// Message struct's field values with the plugin's input channel length and
// capacity, plus any additional data that the plugin might provide through
// implementation of the `ReportingPlugin` interface defined above.
func PopulateReportMsg(pr PluginRunner, msg *message.Message) (err error) {
	if reporter, ok := pr.Plugin().(ReportingPlugin); ok {
		if err = reporter.ReportMsg(msg); err != nil {
			return
		}
	}

	if fRunner, ok := pr.(FilterRunner); ok {
		newIntField(msg, "InChanCapacity", cap(fRunner.InChan()), "count")
		newIntField(msg, "InChanLength", len(fRunner.InChan()), "count")
		newIntField(msg, "MatchChanCapacity", cap(fRunner.MatchRunner().inChan), "count")
		newIntField(msg, "MatchChanLength", len(fRunner.MatchRunner().inChan), "count")
		newIntField(msg, "LeakCount", fRunner.LeakCount(), "count")
		var tmp int64 = 0
		fRunner.MatchRunner().reportLock.Lock()
		if fRunner.MatchRunner().matchSamples > 0 {
			tmp = fRunner.MatchRunner().matchDuration / fRunner.MatchRunner().matchSamples
		}
		fRunner.MatchRunner().reportLock.Unlock()
		newInt64Field(msg, "MatchAvgDuration", tmp, "ns")
	} else if dRunner, ok := pr.(DecoderRunner); ok {
		newIntField(msg, "InChanCapacity", cap(dRunner.InChan()), "count")
		newIntField(msg, "InChanLength", len(dRunner.InChan()), "count")
	}
	msg.SetType("heka.plugin-report")
	return
}

// Generate recycle channel and plugin report messages and put them on the
// provided channel as they're ready.
func (pc *PipelineConfig) reports(reportChan chan *PipelinePack) {
	var (
		f      *message.Field
		pack   *PipelinePack
		msg    *message.Message
		err, e error
	)

	pack = <-pc.reportRecycleChan
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.inputRecycleChan), "count")
	newIntField(msg, "InChanLength", len(pc.inputRecycleChan), "count")
	msg.SetType("heka.input-report")
	newStringField(msg, "name", "inputRecycleChan")
	newStringField(msg, "key", "globals")
	reportChan <- pack

	pack = <-pc.reportRecycleChan
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.injectRecycleChan), "count")
	newIntField(msg, "InChanLength", len(pc.injectRecycleChan), "count")
	msg.SetType("heka.inject-report")
	newStringField(msg, "name", "injectRecycleChan")
	newStringField(msg, "key", "globals")
	reportChan <- pack

	pack = <-pc.reportRecycleChan
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.router.InChan()), "count")
	newIntField(msg, "InChanLength", len(pc.router.InChan()), "count")
	newInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&pc.router.processMessageCount), "count")
	msg.SetType("heka.router-report")
	newStringField(msg, "name", "Router")
	newStringField(msg, "key", "globals")
	reportChan <- pack

	getReport := func(runner PluginRunner) (pack *PipelinePack) {
		pack = <-pc.reportRecycleChan
		if err = PopulateReportMsg(runner, pack.Message); err != nil {
			msg = pack.Message
			f, e = message.NewField("Error", err.Error(), "")
			if e == nil {
				msg.AddField(f)
			}
			msg.SetType("heka.plugin-report")
		}
		return
	}

	pc.inputsLock.Lock()
	for name, runner := range pc.InputRunners {
		pack = getReport(runner)
		newStringField(pack.Message, "name", name)
		newStringField(pack.Message, "key", "inputs")
		reportChan <- pack
	}
	pc.inputsLock.Unlock()

	for _, runner := range pc.allDecoders {
		pack = getReport(runner)
		newStringField(pack.Message, "name", runner.Name())
		newStringField(pack.Message, "key", "decoders")
		reportChan <- pack
	}

	for name, dChan := range pc.decoderChannels {
		pack = <-pc.reportRecycleChan
		msg = pack.Message
		msg.SetType("heka.decoder-pool-report")
		newStringField(msg, "name", fmt.Sprintf("DecoderPool-%s", name))
		newStringField(msg, "key", "decoderPools")
		newIntField(msg, "InChanCapacity", cap(dChan), "count")
		newIntField(msg, "InChanLength", len(dChan), "count")
		reportChan <- pack
	}

	pc.filtersLock.Lock()
	for name, runner := range pc.FilterRunners {
		pack = getReport(runner)
		newStringField(pack.Message, "name", name)
		newStringField(pack.Message, "key", "filters")
		reportChan <- pack
	}
	pc.filtersLock.Unlock()

	for name, runner := range pc.OutputRunners {
		pack = getReport(runner)
		newStringField(pack.Message, "name", name)
		newStringField(pack.Message, "key", "outputs")
		reportChan <- pack
	}
	close(reportChan)
}

// Use type aliases for readability.
type pluginReportDataMap map[string]interface{}
type fullReportDataMap map[string][]pluginReportDataMap

// Generates a single message with a payload that is a string representation
// of the fields data and payload extracted from each running plugin's report
// message and hands the message to the router for delivery.
func (pc *PipelineConfig) allReportsData() (report_type, msg_payload string) {
	var (
		iName, iKey interface{}
		key, name   string
		ok          bool
	)
	reports := make(chan *PipelinePack)
	go pc.reports(reports)

	MISSING := "MISSING"
	data := make(fullReportDataMap)
	for pack := range reports {
		if iKey, ok = pack.Message.GetFieldValue("key"); !ok {
			key = MISSING
		} else if key, ok = iKey.(string); !ok {
			key = MISSING
		}
		if iName, ok = pack.Message.GetFieldValue("name"); !ok {
			name = MISSING
		} else if name, ok = iName.(string); !ok {
			name = MISSING
		}

		pData := make(pluginReportDataMap)
		pData["Name"] = name

		for _, field := range pack.Message.Fields {
			if field.GetName() == "name" || field.GetName() == "key" {
				continue
			}
			valMap := map[string]interface{}{
				"value":          field.GetValue(),
				"representation": field.GetRepresentation(),
			}
			pData[field.GetName()] = valMap
		}

		data[key] = append(data[key], pData)
		pack.Recycle()
	}
	buffer := new(bytes.Buffer)
	enc := json.NewEncoder(buffer)
	enc.Encode(data)

	return "heka.all-report", buffer.String()
}

// Generates a single message with a payload that is a string representation
// of the fields data and payload extracted from each running plugin's report
// message and hands the message to the router for delivery.
func (pc *PipelineConfig) allReportsMsg() {
	report_type, msg_payload := pc.allReportsData()

	pack := pc.PipelinePack(0)
	pack.Message.SetType(report_type)
	pack.Message.SetPayload(msg_payload)
	pc.router.InChan() <- pack
}

func (pc *PipelineConfig) allReportsStdout() {
	report_type, msg_payload := pc.allReportsData()
	pc.log(pc.formatTextReport(report_type, msg_payload))
}

func (pc *PipelineConfig) formatTextReport(report_type, payload string) string {

	header := []string{
		"InChanCapacity", "InChanLength", "MatchChanCapacity", "MatchChanLength",
		"MatchAvgDuration", "ProcessMessageCount", "InjectMessageCount", "Memory",
		"MaxMemory", "MaxInstructions", "MaxOutput", "ProcessMessageAvgDuration",
		"TimerEventAvgDuration",
	}

	///////////

	m := make(map[string]interface{})
	json.Unmarshal([]byte(payload), &m)

	fullReport := make([]string, 0)
	for _, row := range m["reports"].([]interface{}) {
		pluginReport := make([]string, 0)
		pluginReport = append(pluginReport,
			fmt.Sprintf("%s:", (row.(map[string]interface{}))["Plugin"].(string)))
		for _, colname := range header {
			data := row.(map[string]interface{})[colname]
			if data != nil {
				pluginReport = append(pluginReport,
					fmt.Sprintf("    %s: %s",
						colname,
						data.(map[string]interface{})["value"]))
			}
		}

		fullReport = append(fullReport, strings.Join(pluginReport, "\n"))
	}

	stdout_report := fmt.Sprintf("========[%s]========\n%s\n========\n",
		report_type,
		strings.Join(fullReport, "\n"))

	return stdout_report
}

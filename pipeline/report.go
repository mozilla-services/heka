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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/mozilla-services/heka/message"
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

type ReportingDecoder struct {
	name    string
	decoder Decoder
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
		message.NewIntField(msg, "InChanCapacity", cap(fRunner.InChan()), "count")
		message.NewIntField(msg, "InChanLength", len(fRunner.InChan()), "count")
		message.NewIntField(msg, "MatchChanCapacity", cap(fRunner.MatchRunner().inChan), "count")
		message.NewIntField(msg, "MatchChanLength", len(fRunner.MatchRunner().inChan), "count")
		message.NewIntField(msg, "LeakCount", fRunner.LeakCount(), "count")
		var tmp int64 = 0
		fRunner.MatchRunner().reportLock.Lock()
		if fRunner.MatchRunner().matchSamples > 0 {
			tmp = fRunner.MatchRunner().matchDuration / fRunner.MatchRunner().matchSamples
		}
		fRunner.MatchRunner().reportLock.Unlock()
		message.NewInt64Field(msg, "MatchAvgDuration", tmp, "ns")
	} else if dRunner, ok := pr.(DecoderRunner); ok {
		message.NewIntField(msg, "InChanCapacity", cap(dRunner.InChan()), "count")
		message.NewIntField(msg, "InChanLength", len(dRunner.InChan()), "count")
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
	message.NewIntField(msg, "InChanCapacity", cap(pc.inputRecycleChan), "count")
	message.NewIntField(msg, "InChanLength", len(pc.inputRecycleChan), "count")
	msg.SetLogger(HEKA_DAEMON)
	msg.SetType("heka.input-report")
	message.NewStringField(msg, "name", "inputRecycleChan")
	message.NewStringField(msg, "key", "globals")
	reportChan <- pack

	pack = <-pc.reportRecycleChan
	msg = pack.Message
	message.NewIntField(msg, "InChanCapacity", cap(pc.injectRecycleChan), "count")
	message.NewIntField(msg, "InChanLength", len(pc.injectRecycleChan), "count")
	msg.SetLogger(HEKA_DAEMON)
	msg.SetType("heka.inject-report")
	message.NewStringField(msg, "name", "injectRecycleChan")
	message.NewStringField(msg, "key", "globals")
	reportChan <- pack

	pack = <-pc.reportRecycleChan
	msg = pack.Message
	message.NewIntField(msg, "InChanCapacity", cap(pc.router.InChan()), "count")
	message.NewIntField(msg, "InChanLength", len(pc.router.InChan()), "count")
	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&pc.router.processMessageCount), "count")
	msg.SetLogger(HEKA_DAEMON)
	msg.SetType("heka.router-report")
	message.NewStringField(msg, "name", "Router")
	message.NewStringField(msg, "key", "globals")
	reportChan <- pack

	getReport := func(runner PluginRunner) (pack *PipelinePack) {
		pack = <-pc.reportRecycleChan
		if err = PopulateReportMsg(runner, pack.Message); err != nil {
			msg = pack.Message
			f, e = message.NewField("Error", err.Error(), "")
			if e == nil {
				msg.AddField(f)
			}
			msg.SetLogger(HEKA_DAEMON)
			msg.SetType("heka.plugin-report")
		}
		return
	}

	pc.inputsLock.Lock()
	for name, runner := range pc.InputRunners {
		if runner.Transient() {
			continue
		}
		pack = getReport(runner)
		message.NewStringField(pack.Message, "name", name)
		message.NewStringField(pack.Message, "key", "inputs")
		if runner.SynchronousDecode() {
			message.NewStringField(pack.Message, "SynchronousDecode", "true")
		}
		reportChan <- pack
	}
	pc.inputsLock.Unlock()

	for _, runner := range pc.allDecoders {
		pack = getReport(runner)
		message.NewStringField(pack.Message, "name", runner.Name())
		message.NewStringField(pack.Message, "key", "decoders")
		reportChan <- pack
	}

	for _, reportingDecoder := range pc.allSyncDecoders {
		pack = <-pc.reportRecycleChan
		message.NewStringField(pack.Message, "name", reportingDecoder.name)
		message.NewStringField(pack.Message, "key", "decoders")
		pack.Message.SetLogger(HEKA_DAEMON)
		pack.Message.SetType("heka.plugin-report")

		reportChan <- pack
	}

	for _, runner := range pc.allSplitters {
		pack = <-pc.reportRecycleChan
		message.NewStringField(pack.Message, "name", runner.Name())
		message.NewStringField(pack.Message, "key", "splitters")
		pack.Message.SetLogger(HEKA_DAEMON)
		pack.Message.SetType("heka.plugin-report")

		if reporter, hasReports := runner.Splitter().(ReportingPlugin); hasReports {
			if err = reporter.ReportMsg(msg); err != nil {
				if f, e = message.NewField("Error", err.Error(), ""); e == nil {
					msg.AddField(f)
				}
			}
		}

		reportChan <- pack
	}

	for name, encoder := range pc.allEncoders {
		pack = <-pc.reportRecycleChan
		msg = pack.Message
		msg.SetType("heka.plugin-report")
		message.NewStringField(msg, "name", name)
		message.NewStringField(msg, "key", "encoders")
		if reporter, ok := encoder.(ReportingPlugin); ok {
			if err = reporter.ReportMsg(msg); err != nil {
				if f, e = message.NewField("Error", err.Error(), ""); e == nil {
					msg.AddField(f)
				}
			}
		}
		reportChan <- pack
	}

	pc.filtersLock.Lock()
	for name, runner := range pc.FilterRunners {
		pack = getReport(runner)
		message.NewStringField(pack.Message, "name", name)
		message.NewStringField(pack.Message, "key", "filters")
		reportChan <- pack
	}
	pc.filtersLock.Unlock()

	for name, runner := range pc.OutputRunners {
		pack = getReport(runner)
		message.NewStringField(pack.Message, "name", name)
		message.NewStringField(pack.Message, "key", "outputs")
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
		pack.recycle()
	}
	buffer := new(bytes.Buffer)
	enc := json.NewEncoder(buffer)
	enc.Encode(data)

	return "heka.all-report", buffer.String()
}

// Generates a single message with a payload that is a string representation
// of the fields data and payload extracted from each running plugin's report
// message and hands the message to the router for delivery.
func (pc *PipelineConfig) AllReportsMsg() {
	report_type, msg_payload := pc.allReportsData()

	pack, e := pc.PipelinePack(0)
	if e != nil {
		LogError.Println(e.Error())
		return
	}
	pack.Message.SetLogger(HEKA_DAEMON)
	pack.Message.SetType(report_type)
	pack.Message.SetPayload(msg_payload)
	if err := pack.EncodeMsgBytes(); err != nil {
		LogError.Printf("encoding heka.all-report message: %s\n", err.Error())
		pack.recycle()
	} else {
		pc.router.InChan() <- pack
	}

	mempack, e := pc.PipelinePack(0)
	if e != nil {
		LogError.Println(e.Error())
		return
	}
	mempack.Message.SetLogger(HEKA_DAEMON)
	mempack.Message.SetType("heka.memstat")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	message.NewInt64Field(mempack.Message, "HeapSys", int64(m.HeapSys), "B")
	message.NewInt64Field(mempack.Message, "HeapAlloc", int64(m.HeapAlloc), "B")
	message.NewInt64Field(mempack.Message, "HeapIdle", int64(m.HeapIdle), "B")
	message.NewInt64Field(mempack.Message, "HeapInuse", int64(m.HeapInuse), "B")
	message.NewInt64Field(mempack.Message, "HeapReleased", int64(m.HeapReleased), "B")
	message.NewInt64Field(mempack.Message, "HeapObjects", int64(m.HeapObjects), "count")
	if err := mempack.EncodeMsgBytes(); err != nil {
		LogError.Printf("encoding heka.memstat message: %s\n", err.Error())
		mempack.recycle()
	} else {
		pc.router.InChan() <- mempack
	}
}

func (pc *PipelineConfig) allReportsStdout() {
	report_type, msg_payload := pc.allReportsData()
	pc.log(pc.FormatTextReport(report_type, msg_payload))
}

func (pc *PipelineConfig) FormatTextReport(report_type, payload string) string {

	header := []string{
		"InChanCapacity", "InChanLength", "MatchChanCapacity", "MatchChanLength",
		"MatchAvgDuration", "ProcessMessageCount", "InjectMessageCount", "Memory",
		"MaxMemory", "MaxInstructions", "MaxOutput", "ProcessMessageAvgDuration",
		"TimerEventAvgDuration", "SynchronousDecode",
	}

	///////////

	m := make(map[string]interface{})
	json.Unmarshal([]byte(payload), &m)

	fullReport := make([]string, 0)
	categories := []string{"globals", "inputs", "splitters", "decoders", "filters", "outputs", "encoders"}
	for _, cat := range categories {
		fullReport = append(fullReport, fmt.Sprintf("\n====%s====", strings.Title(cat)))
		catReports, ok := m[cat]
		if !ok {
			fullReport = append(fullReport, fmt.Sprintln("NONE"))
			continue
		}
		for _, row := range catReports.([]interface{}) {
			pluginReport := make([]string, 0)
			pluginReport = append(pluginReport,
				fmt.Sprintf("%s:", (row.(map[string]interface{}))["Name"].(string)))
			for _, colname := range header {
				data := row.(map[string]interface{})[colname]
				if data != nil {
					pluginReport = append(pluginReport,
						fmt.Sprintf("    %s: %v",
							colname,
							data.(map[string]interface{})["value"]))
				}
			}

			fullReport = append(fullReport, strings.Join(pluginReport, "\n"))
		}
	}

	stdout_report := fmt.Sprintf("========[%s]========\n%s\n========\n",
		report_type,
		strings.Join(fullReport, "\n"))

	return stdout_report
}

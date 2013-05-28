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
	"fmt"
	"github.com/mozilla-services/heka/message"
	"strings"
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
func newIntField(msg *message.Message, name string, val int) {
	f, err := message.NewField(name, val, message.Field_RAW)
	if err == nil {
		msg.AddField(f)
	}
}

// Convenience function for creating a new int64 field on a message object.
func newInt64Field(msg *message.Message, name string, val int64) {
	f, err := message.NewField(name, val, message.Field_RAW)
	if err == nil {
		msg.AddField(f)
	}
}

// Convenience function for creating and setting a string field called "name"
// on a message object.
func setNameField(msg *message.Message, name string) {
	f, err := message.NewField("name", name, message.Field_RAW)
	if err == nil {
		msg.AddField(f)
	}
}

// Given a PluginRunner and a Message struct, this function will populate the
// Message struct's field values with the plugin's input channel length and
// capacity, plus any additional data that the plugin might provide through
// implementation of the `ReportingPlugin` interface defined above.
func PopulateReportMsg(pr PluginRunner, msg *message.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("'%s' `populateReportMsg` panic: %s", pr.Name(), r)
		}
	}()

	if reporter, ok := pr.Plugin().(ReportingPlugin); ok {
		if err = reporter.ReportMsg(msg); err != nil {
			return
		}
	}

	if fRunner, ok := pr.(FilterRunner); ok {
		newIntField(msg, "InChanCapacity", cap(fRunner.InChan()))
		newIntField(msg, "InChanLength", len(fRunner.InChan()))
		newIntField(msg, "MatchChanCapacity", cap(fRunner.MatchRunner().inChan))
		newIntField(msg, "MatchChanLength", len(fRunner.MatchRunner().inChan))
		var tmp int64 = 0
		if fRunner.MatchRunner().matchSamples > 0 {
			tmp = fRunner.MatchRunner().matchDuration.Nanoseconds() /
				fRunner.MatchRunner().matchSamples
		}
		newInt64Field(msg, "MatcherAvgDuration", tmp)
	} else if dRunner, ok := pr.(DecoderRunner); ok {
		newIntField(msg, "InChanCapacity", cap(dRunner.InChan()))
		newIntField(msg, "InChanLength", len(dRunner.InChan()))
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

	pack = pc.PipelinePack(0)
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.inputRecycleChan))
	newIntField(msg, "InChanLength", len(pc.inputRecycleChan))
	msg.SetType("heka.input-report")
	setNameField(msg, "inputRecycleChan")
	reportChan <- pack

	pack = pc.PipelinePack(0)
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.injectRecycleChan))
	newIntField(msg, "InChanLength", len(pc.injectRecycleChan))
	msg.SetType("heka.inject-report")
	setNameField(msg, "injectRecycleChan")
	reportChan <- pack

	pack = pc.PipelinePack(0)
	msg = pack.Message
	newIntField(msg, "InChanCapacity", cap(pc.router.InChan()))
	newIntField(msg, "InChanLength", len(pc.router.InChan()))
	newInt64Field(msg, "ProcessMessageCount", pc.router.processMessageCount)
	msg.SetType("heka.router-report")
	setNameField(msg, "Router")
	reportChan <- pack

	getReport := func(runner PluginRunner) (pack *PipelinePack) {
		pack = pc.PipelinePack(0)
		if err = PopulateReportMsg(runner, pack.Message); err != nil {
			msg = pack.Message
			f, e = message.NewField("Error", err.Error(), message.Field_RAW)
			if e == nil {
				msg.AddField(f)
			}
			msg.SetType("heka.plugin-report")
		}
		return
	}

	for name, runner := range pc.InputRunners {
		pack = getReport(runner)
		if len(pack.Message.Fields) > 0 || pack.Message.GetPayload() != "" {
			setNameField(pack.Message, name)
			reportChan <- pack
		} else {
			pack.Recycle()
		}
	}

	for _, runner := range pc.allDecoders {
		pack = getReport(runner)
		setNameField(pack.Message, runner.Name())
		reportChan <- pack
	}

	for name, dChan := range pc.decoderChannels {
		pack = pc.PipelinePack(0)
		msg = pack.Message
		msg.SetType("heka.decoder-pool-report")
		setNameField(msg, fmt.Sprintf("DecoderPool-%s", name))
		newIntField(msg, "InChanCapacity", cap(dChan))
		newIntField(msg, "InChanLength", len(dChan))
		reportChan <- pack
	}

	for name, runner := range pc.FilterRunners {
		pack = getReport(runner)
		setNameField(pack.Message, name)
		reportChan <- pack
	}
	for name, runner := range pc.OutputRunners {
		pack = getReport(runner)
		setNameField(pack.Message, name)
		reportChan <- pack
	}
	close(reportChan)
}

// Generates a single message with a payload that is a string representation
// of the fields data and payload extracted from each running plugin's report
// message and hands the message to the router for delivery.
func (pc *PipelineConfig) allReportsMsg() {
	payload := make([]string, 0, 10)
	var iName interface{}
	var name, line string
	var ok bool
	reports := make(chan *PipelinePack)
	go pc.reports(reports)

	MISSING := "MISSING"
	sep := ""
	payload = append(payload, "{\"reports\":[")
	for pack := range reports {
		if iName, ok = pack.Message.GetFieldValue("name"); !ok {
			name = MISSING
		} else if name, ok = iName.(string); !ok {
			name = MISSING
		}
		line = fmt.Sprintf("%s{\"Plugin\":\"%s\"", sep, name)
		sep = ","
		payload = append(payload, line)
		for _, field := range pack.Message.Fields {
			if field.GetName() == "name" {
				continue
			}
			line = fmt.Sprintf(",\"%s\":\"%v\"", field.GetName(), field.GetValue())
			payload = append(payload, line)
		}
		payload = append(payload, "}")
		pack.Recycle()
	}
	payload = append(payload, "]}")

	pack := pc.PipelinePack(0)
	pack.Message.SetType("heka.all-report")
	pack.Message.SetPayload(strings.Join(payload, ""))
	pc.router.InChan() <- pack
}

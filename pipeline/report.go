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
	ReportMsg(msg *message.Message) (err error)
}

func newIntField(msg *message.Message, name string, val int) {
	f, err := message.NewField(name, val, message.Field_RAW)
	if err == nil {
		msg.AddField(f)
	}
}

func setNameField(msg *message.Message, name string) {
	f, err := message.NewField("name", name, message.Field_RAW)
	if err == nil {
		msg.AddField(f)
	}
}

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
	} else if dRunner, ok := pr.(DecoderRunner); ok {
		newIntField(msg, "InChanCapacity", cap(dRunner.InChan()))
		newIntField(msg, "InChanLength", len(dRunner.InChan()))
	}

	if msg.GetType() != "" {
		var f *message.Field
		f, err = message.NewField("Type", msg.GetType(), message.Field_RAW)
		if err != nil {
			return
		}
		msg.AddField(f)
	}
	msg.SetType("heka.plugin-report")
	return
}

// Generate recycle channel and plugin report messages and put them on the
// provided channel.
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
	newIntField(msg, "InChanCapacity", cap(pc.Router().InChan()))
	newIntField(msg, "InChanLength", len(pc.Router().InChan()))
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
	for i, dSet := range pc.DecoderSets {
		for name, runner := range dSet.AllByName() {
			pack = getReport(runner)
			setNameField(pack.Message, fmt.Sprintf("%s-%d", name, i))
			reportChan <- pack
		}
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

//
func (pc *PipelineConfig) allReportsMsg() {
	payload := make([]string, 0, 10)
	var iName interface{}
	var name, line string
	var ok bool
	reports := make(chan *PipelinePack)
	go pc.reports(reports)

	MISSING := "MISSING"
	for pack := range reports {
		if iName, ok = pack.Message.GetFieldValue("name"); !ok {
			name = MISSING
		} else if name, ok = iName.(string); !ok {
			name = MISSING
		}
		line = fmt.Sprintf("%s:", name)
		payload = append(payload, line)
		for _, field := range pack.Message.Fields {
			if field.GetName() == "name" {
				continue
			}
			line = fmt.Sprintf("\t%s:\t%v", field.GetName(), field.GetValue())
			payload = append(payload, line)
		}
		if pack.Message.GetPayload() != "" {
			line = fmt.Sprintf("\tPayload:\t%s", pack.Message.GetPayload())
			payload = append(payload, line)
		}
		payload = append(payload, "")
		pack.Recycle()
	}

	pack := pc.PipelinePack(0)
	pack.Message.SetType("heka.all-report")
	pack.Message.SetPayload(strings.Join(payload, "\n"))
	pc.Router().InChan() <- pack
}

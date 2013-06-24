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
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

var (
	f0, _ = message.NewField("test0", 0, "")
	f1, _ = message.NewField("test1", "one", "")
)

func (f *CounterFilter) ReportMsg(msg *message.Message) (err error) {
	msg.AddField(f0)
	msg.AddField(f1)
	return
}

func (i *UdpInput) ReportMsg(msg *message.Message) (err error) {
	msg.AddField(f0)
	msg.AddField(f1)
	return
}

func ReportSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	origGlobals := Globals
	Globals = func() *GlobalConfigStruct {
		return DefaultGlobals()
	}
	defer func() {
		Globals = origGlobals
	}()

	checkForFields := func(c gs.Context, msg *message.Message) {
		f0Val, ok := msg.GetFieldValue(f0.GetName())
		c.Expect(ok, gs.IsTrue)
		c.Expect(f0Val.(int64), gs.Equals, f0.GetValue().(int64))
		f1Val, ok := msg.GetFieldValue(f1.GetName())
		c.Expect(ok, gs.IsTrue)
		c.Expect(f1Val.(string), gs.Equals, f1.GetValue().(string))
	}

	hasChannelData := func(msg *message.Message) (ok bool) {
		capVal, _ := msg.GetFieldValue("InChanCapacity")
		lenVal, _ := msg.GetFieldValue("InChanLength")
		var i int64
		if i, ok = capVal.(int64); !ok {
			return
		}
		if ok = (i == int64(Globals().PluginChanSize)); !ok {
			return
		}
		if i, ok = lenVal.(int64); !ok {
			return
		}
		ok = (i == int64(0))
		return
	}

	fName := "counter"
	filter := new(CounterFilter)
	fRunner := NewFORunner(fName, filter, nil)
	var err error
	fRunner.matcher, err = NewMatchRunner("Type == ''", "")
	c.Assume(err, gs.IsNil)
	fRunner.matcher.inChan = make(chan *PipelinePack, 10)

	iName := "udp"
	input := new(UdpInput)
	iRunner := NewInputRunner(iName, input, nil)

	c.Specify("`PopulateReportMsg`", func() {
		msg := getTestMessage()

		c.Specify("w/ a filter", func() {
			err := PopulateReportMsg(fRunner, msg)
			c.Assume(err, gs.IsNil)

			c.Specify("invokes `ReportMsg` on the filter", func() {
				checkForFields(c, msg)
			})

			c.Specify("adds the channel data", func() {
				c.Expect(hasChannelData(msg), gs.IsTrue)
			})
		})

		c.Specify("w/ an input", func() {
			err := PopulateReportMsg(iRunner, msg)
			c.Assume(err, gs.IsNil)

			c.Specify("invokes `ReportMsg` on the input", func() {
				checkForFields(c, msg)
			})

			c.Specify("doesn't add any channel data", func() {
				capVal, ok := msg.GetFieldValue("InChanCapacity")
				c.Expect(capVal, gs.IsNil)
				c.Expect(ok, gs.IsFalse)
				lenVal, ok := msg.GetFieldValue("InChanLength")
				c.Expect(lenVal, gs.IsNil)
				c.Expect(ok, gs.IsFalse)
			})
		})
	})

	c.Specify("PipelineConfig", func() {
		pc := NewPipelineConfig(nil)
		// Initialize all of the PipelinePacks that we'll need
		for i := 0; i < Globals().PoolSize; i++ {
			pc.injectRecycleChan <- NewPipelinePack(pc.injectRecycleChan)
		}

		pc.FilterRunners = map[string]FilterRunner{fName: fRunner}
		pc.InputRunners = map[string]InputRunner{iName: iRunner}

		c.Specify("returns full set of accurate reports", func() {
			reportChan := make(chan *PipelinePack)
			go pc.reports(reportChan)

			reports := make(map[string]*PipelinePack)
			for r := range reportChan {
				iName, ok := r.Message.GetFieldValue("name")
				c.Expect(ok, gs.IsTrue)
				name, ok := iName.(string)
				c.Expect(ok, gs.IsTrue)
				c.Expect(name, gs.Not(gs.Equals), "MISSING")
				reports[name] = r
			}
			fReport := reports[fName]
			c.Expect(fReport, gs.Not(gs.IsNil))
			checkForFields(c, fReport.Message)
			c.Expect(hasChannelData(fReport.Message), gs.IsTrue)

			iReport := reports[iName]
			c.Expect(iReport, gs.Not(gs.IsNil))
			checkForFields(c, iReport.Message)

			recycleReport := reports["inputRecycleChan"]
			c.Expect(recycleReport, gs.Not(gs.IsNil))
			capVal, ok := recycleReport.Message.GetFieldValue("InChanCapacity")
			c.Expect(ok, gs.IsTrue)
			c.Expect(capVal.(int64), gs.Equals, int64(Globals().PoolSize))

			injectReport := reports["injectRecycleChan"]
			c.Expect(injectReport, gs.Not(gs.IsNil))
			capVal, ok = injectReport.Message.GetFieldValue("InChanCapacity")
			c.Expect(ok, gs.IsTrue)
			c.Expect(capVal.(int64), gs.Equals, int64(Globals().PoolSize))

			routerReport := reports["Router"]
			c.Expect(routerReport, gs.Not(gs.IsNil))
			c.Expect(hasChannelData(routerReport.Message), gs.IsTrue)
		})
	})
}

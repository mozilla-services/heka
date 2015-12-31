/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/mozilla-services/heka/pipeline"
	ts "github.com/mozilla-services/heka/pipeline/testsupport"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func OutputSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	pConfig := pipeline.NewPipelineConfig(nil)
	inChan := make(chan *pipeline.PipelinePack, 1)
	timer := make(chan time.Time, 1)

	c.Specify("A SandboxOutput", func() {
		output := new(SandboxOutput)
		output.SetPipelineConfig(pConfig)
		conf := output.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../lua/testsupport/output.lua"
		conf.ModuleDirectory = "../lua/modules;../lua/testsupport/modules"
		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		data := "1376389920 debug id=2321 url=example.com item=1"
		pack.Message.SetPayload(data)

		oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
		oth.MockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
		oth.MockOutputRunner.EXPECT().Ticker().Return(timer)

		c.Specify("writes a payload to file", func() {
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			inChan <- pack
			close(inChan)
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Assume(err, gs.IsNil)

			tmpFile, err := os.Open("output.lua.txt")
			defer tmpFile.Close()
			c.Assume(err, gs.IsNil)
			contents, err := ioutil.ReadAll(tmpFile)
			c.Assume(err, gs.IsNil)
			c.Expect(string(contents), gs.Equals, data)
			c.Expect(output.processMessageCount, gs.Equals, int64(1))
			c.Expect(output.processMessageSamples, gs.Equals, int64(1))
			c.Expect(output.processMessageFailures, gs.Equals, int64(0))
		})

		c.Specify("failure processing data", func() {
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			pack.Message.SetPayload("FAILURE")
			pack.BufferedPack = true
			pack.DelivErrChan = make(chan error, 1)
			inChan <- pack
			close(inChan)
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Assume(err, gs.IsNil)
			c.Expect(output.processMessageFailures, gs.Equals, int64(1))
			err = <-pack.DelivErrChan
			c.Expect(err.Error(), gs.Equals, "failure message")
		})

		c.Specify("user abort processing data", func() {
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			e := errors.New("FATAL: user abort")
			pack.Message.SetPayload("USERABORT")
			inChan <- pack
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err.Error(), gs.Equals, e.Error())
		})

		c.Specify("fatal error processing data", func() {
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			pack.Message.SetPayload("FATAL")
			inChan <- pack
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("fatal error in timer_event", func() {
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			timer <- time.Now()
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.Not(gs.IsNil))
		})

		c.Specify("fatal error in shutdown timer_event", func() {
			conf.TimerEventOnShutdown = true
			err := output.Init(conf)
			c.Expect(err, gs.IsNil)
			close(inChan)
			err = output.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})
}

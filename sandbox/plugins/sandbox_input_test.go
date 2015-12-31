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
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/mozilla-services/heka/sandbox"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"runtime"
	"sync"
	"time"
)

func InputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)

	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	tickChan := make(chan time.Time)
	defer func() {
		close(tickChan)
		close(errChan)
		ctrl.Finish()
	}()

	pConfig := NewPipelineConfig(nil)
	c.Specify("A SandboxInput", func() {
		input := new(SandboxInput)
		input.SetPipelineConfig(pConfig)

		ith := new(plugins_ts.InputTestHelper)
		ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
		ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
		ith.PackSupply = make(chan *PipelinePack, 1)
		ith.Pack = NewPipelinePack(ith.PackSupply)
		ith.PackSupply <- ith.Pack

		startInput := func() {
			wg.Add(1)
			go func() {
				errChan <- input.Run(ith.MockInputRunner, ith.MockHelper)
				wg.Done()
			}()
		}

		c.Specify("test a polling input", func() {
			tickChan := time.Tick(10 * time.Millisecond)
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(2)
			ith.MockInputRunner.EXPECT().LogError(fmt.Errorf("failure message"))

			var cnt int
			ith.MockInputRunner.EXPECT().Inject(gomock.Any()).Do(
				func(pack *PipelinePack) {
					switch cnt {
					case 0:
						c.Expect(pack.Message.GetPayload(), gs.Equals, "line 1")
					case 1:
						c.Expect(pack.Message.GetPayload(), gs.Equals, "line 3")
						input.Stop()
					}
					cnt++
					ith.PackSupply <- pack
				}).Times(2)

			config := input.ConfigStruct().(*sandbox.SandboxConfig)
			config.ScriptFilename = "../lua/testsupport/input.lua"

			err := input.Init(config)
			c.Assume(err, gs.IsNil)

			startInput()

			wg.Wait()
			c.Expect(<-errChan, gs.IsNil)
			c.Expect(input.processMessageCount, gs.Equals, int64(2))
			c.Expect(input.processMessageFailures, gs.Equals, int64(1))
			c.Expect(input.processMessageBytes, gs.Equals, int64(72))
		})

		c.Specify("run once input", func() {
			var tickChan <-chan time.Time
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(1)
			ith.MockInputRunner.EXPECT().LogMessage("single run completed")
			ith.MockInputRunner.EXPECT().Inject(gomock.Any()).Do(
				func(pack *PipelinePack) {
					c.Expect(pack.Message.GetPayload(), gs.Equals, "line 1")
					ith.PackSupply <- pack
				}).Times(1)

			config := input.ConfigStruct().(*sandbox.SandboxConfig)
			config.ScriptFilename = "../lua/testsupport/input.lua"

			err := input.Init(config)
			c.Assume(err, gs.IsNil)

			startInput()

			wg.Wait()
			c.Expect(<-errChan, gs.IsNil)
			c.Expect(input.processMessageCount, gs.Equals, int64(1))
			c.Expect(input.processMessageBytes, gs.Equals, int64(36))
		})

		c.Specify("exit with error", func() {
			tickChan := make(chan time.Time)
			defer close(tickChan)
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

			if runtime.GOOS == "windows" {
				ith.MockInputRunner.EXPECT().LogError(fmt.Errorf("process_message() ..\\lua\\testsupport\\input_error.lua:2: boom"))
			} else {
				ith.MockInputRunner.EXPECT().LogError(fmt.Errorf("process_message() ../lua/testsupport/input_error.lua:2: boom"))
			}

			config := input.ConfigStruct().(*sandbox.SandboxConfig)
			config.ScriptFilename = "../lua/testsupport/input_error.lua"

			err := input.Init(config)
			c.Assume(err, gs.IsNil)

			startInput()

			wg.Wait()
			c.Expect(<-errChan, gs.IsNil)
			c.Expect(input.processMessageCount, gs.Equals, int64(0))
			c.Expect(input.processMessageBytes, gs.Equals, int64(0))
		})
	})
}

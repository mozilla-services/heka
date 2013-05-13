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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	"github.com/mozilla-services/heka/sandbox"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"sync"
	"time"
)

type FilterTestHelper struct {
	MockHelper       *MockPluginHelper
	MockFilterRunner *MockFilterRunner
}

func NewFilterTestHelper(ctrl *gomock.Controller) (fth *FilterTestHelper) {
	fth = new(FilterTestHelper)
	fth.MockHelper = NewMockPluginHelper(ctrl)
	fth.MockFilterRunner = NewMockFilterRunner(ctrl)
	return
}

type PanicFilter struct{}

func (p *PanicFilter) Init(config interface{}) (err error) {
	panic("PANICFILTER")
}

func (p *PanicFilter) Run(fr FilterRunner, h PluginHelper) (err error) {
	panic("PANICFILTER")
}

func FiltersSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fth := NewFilterTestHelper(ctrl)
	inChan := make(chan *PipelineCapture, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A SandboxFilter", func() {
		sbFilter := new(SandboxFilter)
		config := sbFilter.ConfigStruct().(*sandbox.SandboxConfig)
		config.ScriptType = "lua"
		config.MemoryLimit = 32000
		config.InstructionLimit = 1000
		config.OutputLimit = 1024

		msg := getTestMessage()
		pack := NewPipelinePack(pConfig.inputRecycleChan)
		pack.Message = msg
		pack.Decoded = true
		plc := &PipelineCapture{Pack: pack}

		c.Specify("Over inject messages from ProcessMessage", func() {
			var timer <-chan time.Time
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("processinject").Times(2)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(2)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(2)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../sandbox/lua/testsupport/processinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			inChan <- plc
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})

		c.Specify("Over inject messages from TimerEvent", func() {
			var timer <-chan time.Time
			timer = time.Tick(time.Duration(1) * time.Millisecond)
			fth.MockFilterRunner.EXPECT().Ticker().Return(timer)
			fth.MockFilterRunner.EXPECT().InChan().Return(inChan)
			fth.MockFilterRunner.EXPECT().Name().Return("processinject").Times(11)
			fth.MockFilterRunner.EXPECT().Inject(pack).Return(true).Times(11)
			fth.MockHelper.EXPECT().PipelinePack(uint(0)).Return(pack).Times(11)
			fth.MockFilterRunner.EXPECT().LogError(fmt.Errorf("exceeded InjectMessage count"))

			config.ScriptFilename = "../sandbox/lua/testsupport/timerinject.lua"
			err := sbFilter.Init(config)
			c.Assume(err, gs.IsNil)
			sbFilter.Run(fth.MockFilterRunner, fth.MockHelper)
		})
	})

	c.Specify("Runner recovers from panic in filter's `Run()` method", func() {
		filter := new(PanicFilter)
		fRunner := NewFORunner("panic", filter)
		var wg sync.WaitGroup
		wg.Add(1)
		fRunner.Start(fth.MockHelper, &wg) // no panic => success
		wg.Wait()
	})
}

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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import (
	"code.google.com/p/gomock/gomock"
	//"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"runtime"
	"time"
)

func ProcessDirectoryInputSpec(c gs.Context) {

	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(config.InputRecycleChan())

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.Decoder = pipelinemock.NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)

	ith.PackSupply <- ith.Pack
	ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).AnyTimes()

	ith.DecodeChan = make(chan *PipelinePack)

	mockDecoderRunner := ith.Decoder.(*pipelinemock.MockDecoderRunner)
	mockDecoderRunner.EXPECT().InChan().Return(ith.DecodeChan).AnyTimes()

	c.Specify("A ProcessDirectoryInput", func() {
		pdiInput := ProcessDirectoryInput{}

		ith.MockInputRunner.EXPECT().Name().Return("logger").AnyTimes()

		// Ignore all messages
		ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
		//ith.MockHelper.EXPECT().PipelineConfig().AnyTimes()

		config := pdiInput.ConfigStruct().(*ProcessDirectoryInputConfig)
		config.ScheduledJobDir = "testsupport/tcollector_unix"

		pConfig := NewPipelineConfig(nil)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig).AnyTimes()

		tickChan := make(chan time.Time, 1)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

		c.Specify("loads scheduled jobs", func() {
			// Clobber the base directory so that we scope tcollector into
			// the testsupport directory located under
			// src/heka/cmd/hekad
			clobber_base_dir, err := os.Getwd()
			c.Expect(err, gs.IsNil)

			Globals().BaseDir = clobber_base_dir

			pdiInput.cron_dir = GetHekaConfigDir(config.ScheduledJobDir)
			err = pdiInput.Init(config)
			c.Assume(err, gs.IsNil)

			go func() {
				pdiInput.Run(ith.MockInputRunner, ith.MockHelper)
			}()

			tickChan <- time.Now()
			<-ith.DecodeChan
			runtime.Gosched()

			pdiInput.Stop()

		})
	})

}

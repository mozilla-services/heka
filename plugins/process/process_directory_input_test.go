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
	"fmt"
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
	ith.DecodeChan = make(chan *PipelinePack)

	c.Specify("A ProcessDirectoryInput", func() {
		pdiInput := ProcessDirectoryInput{}

		// Ignore all messages
		ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
		//ith.MockHelper.EXPECT().PipelineConfig().AnyTimes()

		config := pdiInput.ConfigStruct().(*ProcessDirectoryInputConfig)
		config.ScheduledJobDir = "testsupport/tcollector_unix"

		pConfig := NewPipelineConfig(nil)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig).AnyTimes()

		tickChan := make(chan time.Time, 100)
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
			for i := 0; i < 10; i++ {
				tickChan <- time.Now()
				fmt.Printf("Tickchan ticked\n")
			}

			actual_payloads := []string{}

			fmt.Printf("Start loop\n")
			for x := 0; x < 4; x++ {
				fmt.Printf("iterate: %d\n", x)
				ith.PackSupply <- ith.Pack
				packRef := <-ith.DecodeChan
				c.Expect(ith.Pack, gs.Equals, packRef)
				actual_payloads = append(actual_payloads, *packRef.Message.Payload)
				fPInputName := *packRef.Message.FindFirstField("ProcessInputName")
				c.Expect(fPInputName.ValueString[0], gs.Equals, "blah")
				// Free up the scheduler
				runtime.Gosched()
			}
		})
	})

}

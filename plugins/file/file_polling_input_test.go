/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Chance Zibolski (chance.zibolski@gmail.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func FilePollingInputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)

	tmpFileName := fmt.Sprintf("filepollinginput-test-%d", time.Now().UnixNano())
	tmpFilePath := filepath.Join(os.TempDir(), tmpFileName)

	defer func() {
		ctrl.Finish()
		os.Remove(tmpFilePath)
	}()

	pConfig := NewPipelineConfig(nil)
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	tickChan := make(chan time.Time)

	retPackChan := make(chan *PipelinePack, 2)
	defer close(retPackChan)

	c.Specify("A FilePollingInput", func() {
		input := new(FilePollingInput)

		ith := new(plugins_ts.InputTestHelper)
		ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
		ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

		ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
		ith.PackSupply = make(chan *PipelinePack, 2)
		ith.PackSupply <- ith.Pack
		ith.PackSupply <- ith.Pack

		config := input.ConfigStruct().(*FilePollingInputConfig)
		config.FilePath = tmpFilePath

		c.Specify("That is started", func() {
			startInput := func() {
				wg.Add(1)
				go func() {
					errChan <- input.Run(ith.MockInputRunner, ith.MockHelper)
					wg.Done()
				}()
			}

			inputName := "FilePollingInput"
			ith.MockInputRunner.EXPECT().Name().Return(inputName).AnyTimes()
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			c.Specify("gets updated information when reading a file", func() {

				c.Specify("injects messages into the pipeline when not configured with a decoder", func() {
					ith.MockInputRunner.EXPECT().Inject(ith.Pack).Do(func(pack *PipelinePack) {
						retPackChan <- pack
					}).Times(2)

					err := input.Init(config)
					c.Assume(err, gs.IsNil)

					c.Expect(input.DecoderName, gs.Equals, "")

				})

				c.Specify("sends messages to a decoder when configured with a decoder", func() {
					decoderName := "ScribbleDecoder"
					config.DecoderName = decoderName

					mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)
					mockDecoderRunner.EXPECT().InChan().Return(retPackChan)
					ith.MockHelper.EXPECT().DecoderRunner(decoderName,
						fmt.Sprintf("%s-%s", inputName, decoderName)).Return(mockDecoderRunner, true)

					err := input.Init(config)
					c.Assume(err, gs.IsNil)

					c.Expect(input.DecoderName, gs.Equals, decoderName)
				})

				startInput()

				f, err := os.Create(tmpFilePath)
				c.Expect(err, gs.IsNil)

				_, err = f.Write([]byte("test1"))
				c.Expect(err, gs.IsNil)
				c.Expect(f.Close(), gs.IsNil)

				tickChan <- time.Now()
				pack := <-retPackChan
				c.Expect(pack.Message.GetPayload(), gs.Equals, "test1")

				f, err = os.Create(tmpFilePath)
				c.Expect(err, gs.IsNil)

				_, err = f.Write([]byte("test2"))
				c.Expect(err, gs.IsNil)
				c.Expect(f.Close(), gs.IsNil)

				tickChan <- time.Now()
				pack = <-retPackChan
				c.Expect(pack.Message.GetPayload(), gs.Equals, "test2")

				input.Stop()
				wg.Wait()
				c.Expect(<-errChan, gs.IsNil)
			})
		})

	})

}

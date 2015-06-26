/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Chance Zibolski (chance.zibolski@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
	bytesChan := make(chan []byte, 1)
	tickChan := make(chan time.Time)

	retPackChan := make(chan *PipelinePack, 2)
	defer close(retPackChan)

	c.Specify("A FilePollingInput", func() {
		input := new(FilePollingInput)

		ith := new(plugins_ts.InputTestHelper)
		ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
		ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
		ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)

		config := input.ConfigStruct().(*FilePollingInputConfig)
		config.FilePath = tmpFilePath

		startInput := func(msgCount int) {
			wg.Add(1)
			go func() {
				errChan <- input.Run(ith.MockInputRunner, ith.MockHelper)
				wg.Done()
			}()
		}

		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)
		ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)

		c.Specify("gets updated information when reading a file", func() {

			err := input.Init(config)
			c.Assume(err, gs.IsNil)

			ith.MockInputRunner.EXPECT().NewSplitterRunner("").Return(ith.MockSplitterRunner)
			ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
			ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
			ith.MockSplitterRunner.EXPECT().Done()
			splitCall := ith.MockSplitterRunner.EXPECT().SplitStream(gomock.Any(),
				nil).Return(io.EOF).Times(2)
			splitCall.Do(func(f *os.File, del Deliverer) {
				fBytes, err := ioutil.ReadAll(f)
				if err != nil {
					fBytes = []byte(err.Error())
				}
				bytesChan <- fBytes
			})

			startInput(2)

			f, err := os.Create(tmpFilePath)
			c.Expect(err, gs.IsNil)

			_, err = f.Write([]byte("test1"))
			c.Expect(err, gs.IsNil)
			c.Expect(f.Close(), gs.IsNil)

			tickChan <- time.Now()

			msgBytes := <-bytesChan
			c.Expect(string(msgBytes), gs.Equals, "test1")

			f, err = os.Create(tmpFilePath)
			c.Expect(err, gs.IsNil)

			_, err = f.Write([]byte("test2"))
			c.Expect(err, gs.IsNil)
			c.Expect(f.Close(), gs.IsNil)

			tickChan <- time.Now()
			msgBytes = <-bytesChan
			c.Expect(string(msgBytes), gs.Equals, "test2")

			input.Stop()
			wg.Wait()
			c.Expect(<-errChan, gs.IsNil)
		})

	})

}

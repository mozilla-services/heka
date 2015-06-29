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
#   Ben Bangert (bbangert@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package logstreamer

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	ls "github.com/mozilla-services/heka/logstreamer"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(LogstreamerInputSpec)

	gs.MainGoTest(r, t)
}

type deliverer struct {
	deliver DeliverFunc
}

func (d *deliverer) Deliver(pack *PipelinePack) {
	d.deliver(pack)
}

func (d *deliverer) DeliverFunc() DeliverFunc {
	return d.deliver
}

func (d *deliverer) Done() {
}

func LogstreamerInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	here, _ := os.Getwd()
	dirPath := filepath.Join(here, "../../logstreamer", "testdir", "filehandling/subdir")

	tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests")
	c.Expect(tmpErr, gs.Equals, nil)
	defer func() {
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.IsNil)
	}()

	globals := DefaultGlobals()
	globals.BaseDir = tmpDir
	pConfig := NewPipelineConfig(globals)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

	// Specify localhost, but we're not really going to use the network.
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// Set up mock helper, runner, and pack supply channel.
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.MockDeliverer = pipelinemock.NewMockDeliverer(ctrl)
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)

	c.Specify("A LogstreamerInput", func() {
		lsInput := &LogstreamerInput{pConfig: pConfig}
		lsiConfig := lsInput.ConfigStruct().(*LogstreamerInputConfig)
		lsiConfig.LogDirectory = dirPath
		lsiConfig.FileMatch = `file.log(\.?)(?P<Seq>\d+)?`
		lsiConfig.Differentiator = []string{"logfile"}
		lsiConfig.Priority = []string{"^Seq"}

		c.Specify("w/ no translation map", func() {
			err := lsInput.Init(lsiConfig)
			c.Expect(err, gs.IsNil)
			c.Expect(len(lsInput.plugins), gs.Equals, 1)

			// Create pool of packs.
			numLines := 5 // # of lines in the log file we're parsing.
			packs := make([]*PipelinePack, numLines)
			ith.PackSupply = make(chan *PipelinePack, numLines)
			for i := 0; i < numLines; i++ {
				packs[i] = NewPipelinePack(ith.PackSupply)
				ith.PackSupply <- packs[i]
			}

			c.Specify("reads a log file", func() {
				// Expect InputRunner calls to get InChan and inject outgoing msgs.
				ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
				ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
				ith.MockInputRunner.EXPECT().NewDeliverer("1").Return(ith.MockDeliverer)
				ith.MockInputRunner.EXPECT().NewSplitterRunner("1").Return(
					ith.MockSplitterRunner)
				ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
				ith.MockSplitterRunner.EXPECT().IncompleteFinal().Return(false)
				ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
				ith.MockSplitterRunner.EXPECT().Done()

				getRecCall := ith.MockSplitterRunner.EXPECT().GetRecordFromStream(
					gomock.Any()).Times(numLines)
				line := "boo hoo foo foo"
				getRecCall.Return(len(line), []byte(line), nil)
				getRecCall = ith.MockSplitterRunner.EXPECT().GetRecordFromStream(gomock.Any())
				getRecCall.Return(0, make([]byte, 0), io.EOF)

				deliverChan := make(chan []byte, 1)
				deliverCall := ith.MockSplitterRunner.EXPECT().DeliverRecord(gomock.Any(),
					ith.MockDeliverer).Times(numLines)
				deliverCall.Do(func(record []byte, del Deliverer) {
					deliverChan <- record
				})

				ith.MockDeliverer.EXPECT().Done()

				runOutChan := make(chan error, 1)
				go func() {
					err = lsInput.Run(ith.MockInputRunner, ith.MockHelper)
					runOutChan <- err
				}()

				dur, _ := time.ParseDuration("5s")
				timeout := time.After(dur)
				timed := false
				for x := 0; x < numLines; x++ {
					select {
					case record := <-deliverChan:
						c.Expect(string(record), gs.Equals, line)
					case <-timeout:
						timed = true
						x += numLines
					}
					// Free up the scheduler while we wait for the log file lines
					// to be processed.
					runtime.Gosched()
				}
				lsInput.Stop()
				c.Expect(timed, gs.Equals, false)
				c.Expect(<-runOutChan, gs.Equals, nil)
			})
		})

		c.Specify("with a translation map", func() {
			lsiConfig.Translation = make(ls.SubmatchTranslationMap)
			lsiConfig.Translation["Seq"] = make(ls.MatchTranslationMap)

			c.Specify("allows len 1 translation map for 'missing'", func() {
				lsiConfig.Translation["Seq"]["missing"] = 9999
				err := lsInput.Init(lsiConfig)
				c.Expect(err, gs.IsNil)
			})

			c.Specify("doesn't allow len 1 map for other keys", func() {
				lsiConfig.Translation["Seq"]["missin"] = 9999
				err := lsInput.Init(lsiConfig)
				c.Expect(err, gs.Not(gs.IsNil))
				c.Expect(err.Error(), gs.Equals,
					"A translation map with one entry ('Seq') must be specifying a "+
						"'missing' key.")
			})
		})
	})
}

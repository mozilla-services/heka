/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Victor Ng (vng@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package process

import (
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

func ProcessDirectoryInputSpec(c gs.Context) {

	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.MockDeliverer = pipelinemock.NewMockDeliverer(ctrl)
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)

	ith.PackSupply <- ith.Pack

	err := pConfig.RegisterDefault("NullSplitter")
	c.Assume(err, gs.IsNil)

	c.Specify("A ProcessDirectoryInput", func() {
		pdiInput := ProcessDirectoryInput{}
		pdiInput.SetPipelineConfig(pConfig)

		config := pdiInput.ConfigStruct().(*ProcessDirectoryInputConfig)
		workingDir, err := os.Getwd()
		c.Assume(err, gs.IsNil)
		config.ProcessDir = filepath.Join(workingDir, "testsupport", "processes")

		// `Ticker` is the last thing called during the setup part of the
		// input's `Run` method, so it triggers a waitgroup that tests can
		// wait on when they need to ensure initialization has finished.
		var started sync.WaitGroup
		started.Add(1)
		tickChan := make(chan time.Time, 1)
		ith.MockInputRunner.EXPECT().Ticker().Return(tickChan).Do(
			func() {
				started.Done()
			})

		// Similarly we use a waitgroup to signal when LogMessage has been
		// called to know when reloads have completed. Warning: If you call
		// expectLogMessage with a msg that is never passed to LogMessage and
		// then you call loaded.Wait() then your test will hang and never
		// complete.
		var loaded sync.WaitGroup
		expectLogMessage := func(msg string) {
			loaded.Add(1)
			ith.MockInputRunner.EXPECT().LogMessage(msg).Do(
				func(msg string) {
					loaded.Done()
				})
		}

		// Same name => same content.
		paths := []string{
			filepath.Join(config.ProcessDir, "100", "h0.toml"),
			filepath.Join(config.ProcessDir, "100", "h1.toml"),
			filepath.Join(config.ProcessDir, "200", "h0.toml"),
			filepath.Join(config.ProcessDir, "300", "h1.toml"),
		}

		copyFile := func(src, dest string) {
			inFile, err := os.Open(src)
			c.Assume(err, gs.IsNil)
			outFile, err := os.Create(dest)
			c.Assume(err, gs.IsNil)
			_, err = io.Copy(outFile, inFile)
			c.Assume(err, gs.IsNil)
			inFile.Close()
			outFile.Close()
		}

		err = pdiInput.Init(config)
		c.Expect(err, gs.IsNil)

		for _, p := range paths {
			expectLogMessage("Added: " + p)
		}
		go pdiInput.Run(ith.MockInputRunner, ith.MockHelper)
		defer func() {
			pdiInput.Stop()
			for _, entry := range pdiInput.inputs {
				entry.ir.Input().Stop()
			}
		}()
		started.Wait()

		c.Specify("loads scheduled jobs", func() {
			pathIndex := func(name string) (i int) {
				var p string
				for i, p = range paths {
					if name == p {
						return
					}
				}
				return -1
			}

			for name, entry := range pdiInput.inputs {
				i := pathIndex(name)
				// Make sure each file path got registered.
				c.Expect(i, gs.Not(gs.Equals), -1)
				dirName := filepath.Base(filepath.Dir(name))
				dirInt, err := strconv.Atoi(dirName)
				c.Expect(err, gs.IsNil)
				// And that the ticker interval was read correctly.
				c.Expect(uint(dirInt), gs.Equals, entry.config.TickerInterval)
			}
		})

		c.Specify("discovers and adds a new job", func() {
			// Copy one of the files to register a new process.
			newPath := filepath.Join(config.ProcessDir, "300", "h0.toml")
			copyFile(paths[0], newPath)
			defer func() {
				err := os.Remove(newPath)
				c.Assume(err, gs.IsNil)
			}()

			// Set up expectations and trigger process dir reload.
			expectLogMessage("Added: " + newPath)
			tickChan <- time.Now()
			loaded.Wait()

			// Make sure our plugin was loaded.
			c.Expect(len(pdiInput.inputs), gs.Equals, 5)
			newEntry, ok := pdiInput.inputs[newPath]
			c.Expect(ok, gs.IsTrue)
			c.Expect(newEntry.config.TickerInterval, gs.Equals, uint(300))
		})

		c.Specify("removes a deleted job", func() {
			err := os.Remove(paths[3])
			c.Assume(err, gs.IsNil)
			defer func() {
				copyFile(paths[1], paths[3])
			}()

			// Set up expectations and trigger process dir reload.
			expectLogMessage("Removed: " + paths[3])
			tickChan <- time.Now()
			loaded.Wait()

			// Make sure our plugin was deleted.
			c.Expect(len(pdiInput.inputs), gs.Equals, 3)
		})

		c.Specify("notices a changed job", func() {
			// Overwrite one job w/ a slightly different one.
			copyFile(paths[0], paths[3])
			defer copyFile(paths[1], paths[3])

			// Set up expectations and trigger process dir reload.
			expectLogMessage("Removed: " + paths[3])
			expectLogMessage("Added: " + paths[3])
			tickChan <- time.Now()
			loaded.Wait()

			// Make sure the new config was loaded.
			c.Expect(pdiInput.inputs[paths[3]].config.Command["0"].Args[0], gs.Equals,
				"hello world\n")
		})
	})

}

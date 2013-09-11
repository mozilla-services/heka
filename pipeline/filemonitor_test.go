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

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"encoding/json"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
)

func createLogfileInput(journalName string) (*LogfileInput, *LogfileInputConfig) {
	lfInput := new(LogfileInput)
	lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
	lfiConfig.LogFile = "../testsupport/test-zeus.log"
	lfiConfig.DiscoverInterval = 5
	lfiConfig.StatInterval = 5
	lfiConfig.SeekJournalName = journalName
	return lfInput, lfiConfig
}

func FileMonitorSpec(c gs.Context) {
	config := NewPipelineConfig(nil)
	tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
	c.Expect(tmpErr, gs.Equals, nil)
	origBaseDir := Globals().BaseDir
	Globals().BaseDir = tmpDir
	defer func() {
		Globals().BaseDir = origBaseDir
		tmpErr = os.RemoveAll(tmpDir)
		c.Expect(tmpErr, gs.Equals, nil)
	}()
	journalName := "test-seekjournal"
	journalDir := filepath.Join(tmpDir, "seekjournals")
	tmpErr = os.MkdirAll(journalDir, 0770)
	c.Expect(tmpErr, gs.Equals, nil)
	journalPath := filepath.Join(journalDir, journalName)

	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoder = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)

	c.Specify("saved last read position", func() {

		c.Specify("without a previous journal", func() {
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			lfInput, lfiConfig := createLogfileInput(journalName)

			// Initialize the input test helper
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			dName := "decoder-name"
			lfInput.decoderName = dName
			mockDecoderRunner := NewMockDecoderRunner(ctrl)

			// Create pool of packs.
			numLines := 95 // # of lines in the log file we're parsing.
			packs := make([]*PipelinePack, numLines)
			ith.PackSupply = make(chan *PipelinePack, numLines)
			for i := 0; i < numLines; i++ {
				packs[i] = NewPipelinePack(ith.PackSupply)
				ith.PackSupply <- packs[i]
			}

			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(dName)
			pbcall.Return(mockDecoderRunner, true)
			decodeCall := mockDecoderRunner.EXPECT().InChan().Times(numLines)
			decodeCall.Return(ith.DecodeChan)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for x := 0; x < numLines; x++ {
				_ = <-ith.DecodeChan
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}

			newFM := new(FileMonitor)
			newFM.Init(lfiConfig)
			c.Expect(err, gs.Equals, nil)

			fbytes, _ := json.Marshal(lfInput.Monitor)
			json.Unmarshal(fbytes, &newFM)
			c.Expect(newFM.seek, gs.Equals, int64(28950))
		})

		c.Specify("with a previous journal initializes with a seek value", func() {
			lfInput, lfiConfig := createLogfileInput(journalName)

			journalData := `{"last_hash":"f0b60af7f2cb35c3724151422e2f999af6e21fc0","last_start":28650,"last_len":300,"seek":28950}`

			journal, journalErr := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR, 0660)
			c.Expect(journalErr, gs.Equals, nil)

			journal.WriteString(journalData)
			journal.Close()

			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			c.Expect(err, gs.IsNil)

			// # bytes should be set to what's in the journal data
			c.Expect(lfInput.Monitor.seek, gs.Equals, int64(28950))
		})

		c.Specify("resets last read position to 0 if hash doesn't match", func() {
			lfInput, lfiConfig := createLogfileInput(journalName)
			lfiConfig.ResumeFromStart = true

			journalData := `{"last_hash":"xxxxx","last_start":28650,"last_len":300,"seek":28950}`

			journal, journalErr := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR, 0660)
			c.Expect(journalErr, gs.Equals, nil)

			journal.WriteString(journalData)
			journal.Close()

			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			// # bytes should be set to what's in the journal data
			c.Expect(lfInput.Monitor.seek, gs.Equals, int64(0))
		})

		c.Specify("resets last read position to end of file if hash doesn't match", func() {
			lfInput, lfiConfig := createLogfileInput(journalName)
			lfiConfig.ResumeFromStart = false

			journalData := `{"last_hash":"xxxxx","last_start":28650,"last_len":300,"seek":28950}`

			journal, journalErr := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR, 0660)
			c.Expect(journalErr, gs.Equals, nil)

			journal.WriteString(journalData)
			journal.Close()

			err := lfInput.Init(lfiConfig)

			c.Expect(err, gs.IsNil)

			// # bytes should be set to what's in the journal data
			c.Expect(lfInput.Monitor.seek, gs.Equals, int64(28950))
		})

	})

	c.Specify("filemonitor generates expected journal path", func() {
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.LogFile = filepath.Join("..", "testsupport", "test-zeus.log")
		lfiConfig.DiscoverInterval = 5
		lfiConfig.StatInterval = 5

		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.Equals, nil)
		clean := filepath.Join(journalDir, "___testsupport_test-zeus_log")
		c.Expect(lfInput.Monitor.seekJournalPath, gs.Equals, clean)
	})

}

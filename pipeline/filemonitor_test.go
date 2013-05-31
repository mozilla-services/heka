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
	"fmt"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"time"
)

func fix_ctime(logfile_name string) bool {
	// Truncate a file by 0 bytes to just update the ctime
	var tmpfile *os.File
	var err error
	var offset int64
	var ctime, btime int64
	ctime, _ = current_ctime(logfile_name)
	btime, _ = current_btime(logfile_name)

	if ctime == btime {
		time.Sleep(10 * time.Millisecond)
		if tmpfile, err = os.OpenFile(logfile_name, os.O_RDWR|os.O_APPEND,
			0660); err != nil {
			log.Println(err.Error())
			return false
		}
		offset, err = tmpfile.Seek(0, os.SEEK_END)
		tmpfile.Truncate(offset)
		tmpfile.Close()

		ctime, _ = current_ctime(logfile_name)
		btime, _ = current_btime(logfile_name)
		log.Printf("Final btime/ctime: [%d][%d]\n", btime, ctime)
		return btime != ctime
	}
	return true
}

func createLogfileInput(journal_name string) (*LogfileInput, *LogfileInputConfig, bool) {
	logfile_name := "../testsupport/test-zeus.log"

	lfInput := new(LogfileInput)
	lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
	lfiConfig.LogFile = logfile_name
	lfiConfig.DiscoverInterval = 1
	lfiConfig.StatInterval = 1
	lfiConfig.SeekJournal = journal_name
	// Remove any journal that may exist
	os.Remove(path.Clean(journal_name))

	if !fix_ctime(logfile_name) {
		return nil, nil, false
	}
	return lfInput, lfiConfig, true
}

func FileMonitorSpec(c gs.Context) {

	tmp_file, tmp_err := ioutil.TempFile("", "")
	c.Expect(tmp_err, gs.Equals, nil)
	journal_name := tmp_file.Name()
	tmp_file.Close()
	logfile_name := "../testsupport/test-zeus.log"

	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
	ith.Decoders[message.Header_PROTOCOL_BUFFER] = NewMockDecoderRunner(ctrl)
	ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)

	c.Specify("saved last read position", func() {

		c.Specify("without a previous journal", func() {
			lfInput, lfiConfig, ok := createLogfileInput(journal_name)

			// This will fail if btime/ctime is implemented the same
			// way by the underlying OS
			c.Expect(ok, gs.Equals, true)

			// Initialize the input test helper
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			dName := "decoder-name"
			lfInput.decoderNames = []string{dName}
			mockDecoderRunner := NewMockDecoderRunner(ctrl)
			mockDecoder := NewMockDecoder(ctrl)

			// Create pool of packs.
			numLines := 95 // # of lines in the log file we're parsing.
			packs := make([]*PipelinePack, numLines)
			ith.PackSupply = make(chan *PipelinePack, numLines)
			for i := 0; i < numLines; i++ {
				packs[i] = NewPipelinePack(ith.PackSupply)
				ith.PackSupply <- packs[i]
			}

			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Inject(gomock.Any()).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)
			pbcall := ith.MockDecoderSet.EXPECT().ByName(dName)
			pbcall.Return(mockDecoderRunner, true)
			mockDecoderRunner.EXPECT().Decoder().Return(mockDecoder)
			decodeCall := mockDecoder.EXPECT().Decode(gomock.Any()).Times(numLines)
			decodeCall.Return(nil)
			go func() {
				err = lfInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			for len(ith.PackSupply) > 0 {
				// Free up the scheduler while we wait for the log file lines
				// to be processed.
				runtime.Gosched()
			}

			newFM := new(FileMonitor)
			newFM.Init(logfile_name, 5, 5, journal_name)
			err = newFM.setupJournalling()
			c.Expect(err, gs.Equals, nil)

			fbytes, _ := json.Marshal(newFM)
			json.Unmarshal(fbytes, &newFM)
			c.Expect(newFM.seek[logfile_name], gs.Equals, int64(28950))
		})

		c.Specify("with a previous journal initializes with a seek value", func() {
			lfInput, lfiConfig, ok := createLogfileInput(journal_name)

			// This will fail if btime/ctime is implemented the same
			//way by the underlying OS
			c.Expect(ok, gs.Equals, true)

			journal_data := `{"birth_times":{"../testsupport/test-zeus.log":%d},"seek":{"../testsupport/test-zeus.log":28950}}`
			btime, _ := current_btime("../testsupport/test-zeus.log")
			journal_data = fmt.Sprintf(journal_data, btime)

			journal, journal_err := os.OpenFile(journal_name,
				os.O_CREATE|os.O_RDWR, 0660)
			c.Expect(journal_err, gs.Equals, nil)

			journal.WriteString(journal_data)
			journal.Close()

			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			err = lfInput.Monitor.setupJournalling()
			c.Expect(err, gs.IsNil)

			// # bytes should be set to what's in the journal data
			c.Expect(lfInput.Monitor.seek[logfile_name], gs.Equals, int64(28950))
		})

		c.Specify("resets last read position to 0 if birthtime doesn't match", func() {
			lfInput, lfiConfig, ok := createLogfileInput(journal_name)
			c.Expect(ok, gs.Equals, true)
			journal_data := `{"birth_times":{"../testsupport/test-zeus.log":84328423},"seek":{"../testsupport/test-zeus.log":28950}}`
			journal, journal_err := os.OpenFile(journal_name,
				os.O_CREATE|os.O_RDWR, 0660)
			c.Expect(journal_err, gs.Equals, nil)

			journal.WriteString(journal_data)
			journal.Close()

			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			// # bytes should be set to what's in the journal data
			c.Expect(lfInput.Monitor.seek[logfile_name], gs.Equals, int64(0))
		})

	})

}

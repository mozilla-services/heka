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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package file

import (
	"bytes"
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func createIncompleteLogfileInput(journalName string) (*LogfileInput, *LogfileInputConfig) {
	lfInput := new(LogfileInput)
	lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
	lfiConfig.LogFile = filepath.Join("..", "testsupport", "test-zeus-incomplete.log")
	lfiConfig.DiscoverInterval = 5
	lfiConfig.StatInterval = 5
	lfiConfig.SeekJournalName = journalName
	return lfInput, lfiConfig
}

func LogfileInputSpec0(c gs.Context) {
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

	c.Specify("A LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.IsNil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "test-seekjournal"
		lfiConfig.LogFile = "../testsupport/test-zeus.log"
		lfiConfig.Logger = "zeus"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)
		mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 95 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockInputRunner.EXPECT().Name().Return("LogfileInput")
			pbcall := ith.MockHelper.EXPECT().DecoderRunner(lfiConfig.Decoder, "LogfileInput_"+lfiConfig.Decoder)
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
			lfInput.Stop()

			fileBytes, err := ioutil.ReadFile(lfiConfig.LogFile)
			c.Expect(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")
			for i, line := range lines {
				if line == "" {
					continue
				}
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line+"\n")
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "zeus")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f0b60af7f2cb35c3724151422e2f999af6e21fc0","last_len":300,"last_start":28650,"seek":28950}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})

		c.Specify("uses the filename as the default logger name", func() {
			lfInput := new(LogfileInput)
			lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
			lfiConfig.LogFile = "../testsupport/test-zeus.log"

			lfiConfig.DiscoverInterval = 1
			lfiConfig.StatInterval = 1
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.Equals, nil)

			c.Expect(lfInput.Monitor.logger_ident,
				gs.Equals,
				lfiConfig.LogFile)
		})
	})

	c.Specify("A Regex LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "regex-seekjournal"
		lfiConfig.LogFile = "../testsupport/test-zeus.log"
		lfiConfig.Logger = "zeus"
		lfiConfig.ParserType = "regexp"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1

		mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 95 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("doesn't whack default RegexpParser delimiter", func() {
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)
			parser := lfInput.Monitor.parser.(*RegexpParser)
			buf := bytes.NewBuffer([]byte("split\nhere"))
			n, r, err := parser.Parse(buf)
			c.Expect(n, gs.Equals, len("split\n"))
			c.Expect(string(r), gs.Equals, "split")
			c.Expect(err, gs.IsNil)
		})

		c.Specify("keeps captures in the record text", func() {
			lfiConfig.Delimiter = "(\n)"
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)
			parser := lfInput.Monitor.parser.(*RegexpParser)
			buf := bytes.NewBuffer([]byte("split\nhere"))
			n, r, err := parser.Parse(buf)
			c.Expect(n, gs.Equals, len("split\n"))
			c.Expect(string(r), gs.Equals, "split\n")
			c.Expect(err, gs.IsNil)
		})

		c.Specify("reads a log file", func() {
			lfiConfig.Delimiter = "(\n)"
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockInputRunner.EXPECT().Name().Return("LogfileInput")
			pbcall := ith.MockHelper.EXPECT().DecoderRunner(lfiConfig.Decoder, "LogfileInput_"+lfiConfig.Decoder)
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
			lfInput.Stop()

			fileBytes, err := ioutil.ReadFile(lfiConfig.LogFile)
			c.Expect(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")
			for i, line := range lines {
				if line == "" {
					continue
				}
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line+"\n")
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "zeus")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f0b60af7f2cb35c3724151422e2f999af6e21fc0","last_len":300,"last_start":28650,"seek":28950}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A Regex Multiline LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "multiline-seekjournal"
		lfiConfig.LogFile = "../testsupport/multiline.log"
		lfiConfig.Logger = "multiline"
		lfiConfig.ParserType = "regexp"
		lfiConfig.Delimiter = "\n(\\d{4}-\\d{2}-\\d{2})"
		lfiConfig.DelimiterLocation = "start"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 4 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and inject outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			// Expect calls to get decoder and decode each message. Since the
			// decoding is a no-op, the message payload will be the log file
			// line, unchanged.
			ith.MockInputRunner.EXPECT().Name().Return("LogfileInput")
			pbcall := ith.MockHelper.EXPECT().DecoderRunner(lfiConfig.Decoder, "LogfileInput_"+lfiConfig.Decoder)
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
			lfInput.Stop()

			lines := []string{
				"2012-07-13 18:48:01 debug    readSocket()",
				"2012-07-13 18:48:21 info     Processing queue id 3496 -> subm id 2817 from site ms",
				"2012-07-13 18:48:25 debug    line0\nline1\nline2",
				"2012-07-13 18:48:26 debug    readSocket()",
			}
			for i, line := range lines {
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line)
				c.Expect(packs[i].Message.GetLogger(), gs.Equals, "multiline")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"39e4c3e6e9c88a794b3e7c91c155682c34cf1a4a","last_len":41,"last_start":172,"seek":214}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A message.proto LogFileInput", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "protobuf-seekjournal"
		lfiConfig.LogFile = "../testsupport/protobuf.log"
		lfiConfig.ParserType = "message.proto"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = "decoder-name"
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.IsNil)

		mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

		// Create pool of packs.
		numLines := 7 // # of lines in the log file we're parsing.
		packs := make([]*PipelinePack, numLines)
		ith.PackSupply = make(chan *PipelinePack, numLines)
		for i := 0; i < numLines; i++ {
			packs[i] = NewPipelinePack(ith.PackSupply)
			ith.PackSupply <- packs[i]
		}

		c.Specify("reads a log file", func() {
			// Expect InputRunner calls to get InChan and decode outgoing msgs
			ith.MockInputRunner.EXPECT().LogError(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).AnyTimes()
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply).Times(numLines)
			ith.MockInputRunner.EXPECT().Name().Return("LogfileInput")
			pbcall := ith.MockHelper.EXPECT().DecoderRunner(lfiConfig.Decoder, "LogfileInput_"+lfiConfig.Decoder)
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
			lfInput.Stop()

			lines := []int{36230, 41368, 42310, 41343, 37171, 56727, 46492}
			for i, line := range lines {
				c.Expect(len(packs[i].MsgBytes), gs.Equals, line)
				err = proto.Unmarshal(packs[i].MsgBytes, packs[i].Message)
				c.Expect(err, gs.IsNil)
				c.Expect(packs[i].Message.GetType(), gs.Equals, "hekabench")
			}

			// Wait for the file update to hit the disk; better suggestions are welcome
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			journalData := []byte(`{"last_hash":"f67dc6bbbbb6a91b59e661b6170de50c96eab100","last_len":46499,"last_start":255191,"seek":301690}`)
			journalFile, err := ioutil.ReadFile(filepath.Join(tmpDir, "seekjournals", lfiConfig.SeekJournalName))
			c.Expect(err, gs.IsNil)
			c.Expect(bytes.Compare(journalData, journalFile), gs.Equals, 0)
		})
	})

	c.Specify("A message.proto LogFileInput no decoder", func() {
		tmpDir, tmpErr := ioutil.TempDir("", "hekad-tests-")
		c.Expect(tmpErr, gs.Equals, nil)
		origBaseDir := Globals().BaseDir
		Globals().BaseDir = tmpDir
		defer func() {
			Globals().BaseDir = origBaseDir
			tmpErr = os.RemoveAll(tmpDir)
			c.Expect(tmpErr, gs.Equals, nil)
		}()
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
		lfiConfig.SeekJournalName = "protobuf-seekjournal"
		lfiConfig.LogFile = "../testsupport/protobuf.log"
		lfiConfig.ParserType = "message.proto"
		lfiConfig.UseSeekJournal = true
		lfiConfig.Decoder = ""
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1
		err := lfInput.Init(lfiConfig)
		c.Expect(err, gs.Not(gs.IsNil))
	})
}

func LogfileInputSpec1(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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
	journalDir := filepath.Join(tmpDir, "seekjournal")
	tmpErr = os.MkdirAll(journalDir, 0770)
	c.Expect(tmpErr, gs.Equals, nil)

	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(config.InputRecycleChan())

	// Specify localhost, but we're not really going to use the network
	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)

	c.Specify("A LogfileInput", func() {
		c.Specify("save the seek position of the last complete logline", func() {
			lfInput, lfiConfig := createIncompleteLogfileInput(journalName)

			// Initialize the input test helper
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			dName := "decoder-name"
			lfInput.decoderName = dName
			mockDecoderRunner := pipelinemock.NewMockDecoderRunner(ctrl)

			// Create pool of packs.
			numLines := 4 // # of lines in the log file we're parsing.
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
			ith.MockInputRunner.EXPECT().Name().Return("LogfileInput")
			pbcall := ith.MockHelper.EXPECT().DecoderRunner(dName, "LogfileInput_"+dName)
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

			// Check that the persisted hashcode is from the last
			// complete log line
			expected_lastline := `10.1.1.4 plinko-565.byzantium.mozilla.com user3 [15/Mar/2013:12:20:27 -0700] "GET /1.1/user3/storage/passwords?newer=1356237662.44&full=1 HTTP/1.1" 200 1396 "-" "Firefox/20.0.1 FxSync/1.22.0.201304.desktop" "-" "ssl: SSL_RSA_WITH_RC4_128_SHA, version=TLSv1, bits=128" node_s:0.047167 req_s:0.047167 retries:0 req_b:446 "c_l:-"` + "\n"

			c.Expect((strings.IndexAny(string(fbytes),
				sha1_hexdigest(expected_lastline)) > -1), gs.IsTrue)

			json.Unmarshal(fbytes, &newFM)

			if runtime.GOOS == "windows" {
				c.Expect(newFM.seek, gs.Equals, int64(1253))
			} else {
				c.Expect(newFM.seek, gs.Equals, int64(1249))
			}

			lfInput.Stop()
		})
	})

	c.Specify("A LogfileDirectoryManagerInput", func() {
		c.Specify("empty file name", func() {
			var err error

			ldm := new(LogfileDirectoryManagerInput)
			conf := ldm.ConfigStruct().(*LogfileInputConfig)
			conf.LogFile = ""
			err = ldm.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "A logfile name must be specified.")
		})

		c.Specify("glob in file name", func() {
			var err error

			ldm := new(LogfileDirectoryManagerInput)
			conf := ldm.ConfigStruct().(*LogfileInputConfig)
			conf.LogFile = "../testsupport/*.log"
			err = ldm.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "Globs are not allowed in the file name: *.log")
		})

		// Note: Testing the actual functionality (spinning up new plugins within Heka)
		// is a manual process.
	})
}

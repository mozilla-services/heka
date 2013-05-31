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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"runtime"
	"strings"
)

func LogfileSpec(c gs.Context) {
	logfile_name := "../testsupport/test-zeus.log"
	//shortlog_name := "../testsupport/test-zeus.short.log"

	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
	ith.Decoders[message.Header_PROTOCOL_BUFFER] = NewMockDecoderRunner(ctrl)
	ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)

	c.Specify("A LogFileInput", func() {
		lfInput := new(LogfileInput)
		lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)

		lfiConfig.LogFile = logfile_name
		lfiConfig.DiscoverInterval = 1
		lfiConfig.StatInterval = 1

		// setup the seekjournal
		tmpfile, err := ioutil.TempFile("", "")
		c.Expect(err, gs.Equals, nil)
		lfiConfig.SeekJournal = tmpfile.Name()
		tmpfile.Close()

		err = lfInput.Init(lfiConfig)
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

		c.Specify("reads a log file", func() {
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
			fileBytes, err := ioutil.ReadFile(lfiConfig.LogFile)
			c.Expect(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")
			for i, line := range lines {
				if line == "" {
					continue
				}
				c.Expect(packs[i].Message.GetPayload(), gs.Equals, line+"\n")
			}
		})

		/* These tests need to restructure the testcode as the input test helper can't be reused easily.

		   c.Specify("reads a logfile from the start seek is past EOF", func() {
		   })

		   c.Specify("handles log rotation", func() {
		   })
		*/
	})
}

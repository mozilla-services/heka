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
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
)

func createIncompleteLogfileInput(journal_name string) (*LogfileInput, *LogfileInputConfig) {

	lfInput := new(LogfileInput)
	lfiConfig := lfInput.ConfigStruct().(*LogfileInputConfig)
	lfiConfig.LogFile = "../testsupport/test-zeus-incomplete.log"
	lfiConfig.DiscoverInterval = 5
	lfiConfig.StatInterval = 5
	lfiConfig.SeekJournal = journal_name
	// Remove any journal that may exist
	os.Remove(path.Clean(journal_name))

	return lfInput, lfiConfig
}

func LogfileInputSpec(c gs.Context) {
	tmp_file, tmp_err := ioutil.TempFile("", "")
	c.Expect(tmp_err, gs.Equals, nil)
	journal_name := tmp_file.Name()
	tmp_file.Close()

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

	c.Specify("LogfileInput", func() {
		c.Specify("save the seek position of the last complete logline", func() {
			lfInput, lfiConfig := createIncompleteLogfileInput(journal_name)

			// Initialize the input test helper
			err := lfInput.Init(lfiConfig)
			c.Expect(err, gs.IsNil)

			dName := "decoder-name"
			lfInput.decoderNames = []string{dName}
			mockDecoderRunner := NewMockDecoderRunner(ctrl)
			mockDecoder := NewMockDecoder(ctrl)

			// Create pool of packs.
			numLines := 4 // # of lines in the log file we're parsing.
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
			newFM.Init(lfiConfig)
			c.Expect(err, gs.Equals, nil)

			fbytes, _ := json.Marshal(lfInput.Monitor)

			// Check that the persisted hashcode is from the last
			// complete log line
			expected_lastline := `10.1.1.4 plinko-565.byzantium.mozilla.com user3 [15/Mar/2013:12:20:27 -0700] "GET /1.1/user3/storage/passwords?newer=1356237662.44&full=1 HTTP/1.1" 200 1396 "-" "Firefox/20.0.1 FxSync/1.22.0.201304.desktop" "-" "ssl: SSL_RSA_WITH_RC4_128_SHA, version=TLSv1, bits=128" node_s:0.047167 req_s:0.047167 retries:0 req_b:446 "c_l:-"` + "\n"

			c.Expect((strings.IndexAny(string(fbytes),
				sha1_hexdigest(expected_lastline)) > -1), gs.IsTrue)

			json.Unmarshal(fbytes, &newFM)

			c.Expect(newFM.seek, gs.Equals, int64(1249))
		})

	})

}

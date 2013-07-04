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
	"github.com/mozilla-services/heka/message"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func HttpInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	config := NewPipelineConfig(nil)
	config.hostname = "somehostname"
	ith := new(InputTestHelper)
	ith.Msg = getTestMessage()
	ith.Pack = NewPipelinePack(config.inputRecycleChan)

	// set up mock helper, decoder set, and packSupply channel
	ith.MockHelper = NewMockPluginHelper(ctrl)
	ith.MockInputRunner = NewMockInputRunner(ctrl)
	ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
	ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
	mockDecoder := NewMockDecoder(ctrl)

	ith.PackSupply = make(chan *PipelinePack, 1)
	ith.DecodeChan = make(chan *PipelinePack)
	ith.MockDecoderSet = NewMockDecoderSet(ctrl)

	c.Specify("HttpInput", func() {
		c.Specify("does something", func() {
			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)
			ith.MockInputRunner.EXPECT().Ticker()
			ith.MockHelper.EXPECT().PipelineConfig().Return(config)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)

			mockDecoderRunner := ith.Decoders[message.Header_JSON].(*MockDecoderRunner)

			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)

			// Force a mock decoder runner to be returned
			dset := ith.MockDecoderSet.EXPECT().ByName("JsonDecoder")
			dset.Return(ith.Decoders[message.Header_JSON], true)

			// Force a decoder to be available
			mockDecoderRunner.EXPECT().Decoder().Return(mockDecoder)

			// Expect the decoder to be invoked once and return
			// nothing as we want it to be a no-op
			mockDecoder.EXPECT().Decode(ith.Pack).Return(nil)

			// The pack should be injected into the input runner at
			// the end
			ith.MockInputRunner.EXPECT().Inject(ith.Pack)

			httpInput := HttpInput{}
			config := httpInput.ConfigStruct().(*HttpInputConfig)

			config.Url = "http://localhost:8000/"
			config.TickerInterval = 1
			err := httpInput.Init(config)
			c.Assume(err, gs.IsNil)

			json_post := `{"uuid": "xxBI3zyeXU+spG8Uiveumw==", "timestamp": 1372966886023588, "hostname": "Victors-MacBook-Air.local", "pid": 40183, "fields": [{"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_priority", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_ident", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_facility", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_options", "value_string": [""]}], "logger": "", "env_version": "0.8", "type": "cef", "payload": "Jul 04 15:41:26 Victors-MacBook-Air.local CEF:0|mozilla|weave|3|xx\\\\|x|xx\\\\|x|5|cs1Label=requestClientApplication cs1=MySuperBrowser requestMethod=GET request=/ src=127.0.0.1 dest=127.0.0.1 suser=none", "severity": 6}'`

			go func() {
				err = httpInput.Run(ith.MockInputRunner, ith.MockHelper)
				c.Expect(err, gs.IsNil)
			}()
			ith.PackSupply <- ith.Pack

			httpInput.Monitor.dataChan <- []byte(json_post)
		})
	})

}

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
	"time"
)

func HttpInputSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)

	json_post := `{"uuid": "xxBI3zyeXU+spG8Uiveumw==", "timestamp": 1372966886023588, "hostname": "Victors-MacBook-Air.local", "pid": 40183, "fields": [{"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_priority", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_ident", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_facility", "value_string": [""]}, {"representation": "", "value_type": "STRING", "name": "cef_meta.syslog_options", "value_string": [""]}], "logger": "", "env_version": "0.8", "type": "cef", "payload": "Jul 04 15:41:26 Victors-MacBook-Air.local CEF:0|mozilla|weave|3|xx\\\\|x|xx\\\\|x|5|cs1Label=requestClientApplication cs1=MySuperBrowser requestMethod=GET request=/ src=127.0.0.1 dest=127.0.0.1 suser=none", "severity": 6}'`

	c.Specify("A HttpInput", func() {

		httpInput := HttpInput{}

		c.Specify("honors time ticker to flush", func() {
			ith := new(InputTestHelper)
			ith.MockHelper = NewMockPluginHelper(ctrl)
			ith.MockInputRunner = NewMockInputRunner(ctrl)
			startInput := func() {
				go func() {
					err := httpInput.Run(ith.MockInputRunner, ith.MockHelper)
					c.Expect(err, gs.IsNil)
				}()
			}

			ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
			ith.PackSupply = make(chan *PipelinePack, 1)
			ith.PackSupply <- ith.Pack

			ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
			ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
			ith.MockDecoderSet = NewMockDecoderSet(ctrl)

			// Spin up a http server
			server, err := ts.NewOneHttpServer(json_post, "localhost", 9876)
			c.Expect(err, gs.IsNil)
			go server.Start("/")
			time.Sleep(10 * time.Millisecond)

			config := httpInput.ConfigStruct().(*HttpInputConfig)
			config.DecoderName = "JsonDecoder"
			config.Url = "http://localhost:9876/"
			tickChan := make(chan time.Time)

			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)

			ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

			mockDecoderRunner := ith.Decoders[message.Header_JSON].(*MockDecoderRunner)

			// Stub out the DecoderRunner input channel so that we can
			// inspect bytes later on
			dRunnerInChan := make(chan *PipelinePack, 1)
			mockDecoderRunner.EXPECT().InChan().Return(dRunnerInChan)

			dset := ith.MockDecoderSet.EXPECT().ByName("JsonDecoder")
			dset.Return(ith.Decoders[message.Header_JSON], true)

			err = httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()

			tickChan <- time.Now()

			// We need for the pipeline to finish up
			time.Sleep(50 * time.Millisecond)
		})

		c.Specify("short circuits packs into the router", func() {
			ith := new(InputTestHelper)
			ith.MockHelper = NewMockPluginHelper(ctrl)
			ith.MockInputRunner = NewMockInputRunner(ctrl)
			startInput := func() {
				go func() {
					err := httpInput.Run(ith.MockInputRunner, ith.MockHelper)
					c.Expect(err, gs.IsNil)
				}()
			}
			ith.Pack = NewPipelinePack(pConfig.inputRecycleChan)
			ith.PackSupply = make(chan *PipelinePack, 1)
			ith.PackSupply <- ith.Pack

			ith.Decoders = make([]DecoderRunner, int(message.Header_JSON+1))
			ith.Decoders[message.Header_JSON] = NewMockDecoderRunner(ctrl)
			ith.MockDecoderSet = NewMockDecoderSet(ctrl)
			config := httpInput.ConfigStruct().(*HttpInputConfig)
			config.Url = "http://localhost:9876/"
			tickChan := make(chan time.Time)

			ith.MockInputRunner.EXPECT().LogMessage(gomock.Any()).Times(2)
			ith.MockHelper.EXPECT().DecoderSet().Return(ith.MockDecoderSet)

			ith.MockHelper.EXPECT().PipelineConfig().Return(pConfig)
			ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
			ith.MockInputRunner.EXPECT().Ticker().Return(tickChan)

			err := httpInput.Init(config)
			c.Assume(err, gs.IsNil)
			startInput()

			tickChan <- time.Now()

			// We need for the pipeline to finish up
			time.Sleep(50 * time.Millisecond)
		})

	})
}

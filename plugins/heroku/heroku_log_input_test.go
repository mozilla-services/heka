/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# ***** END LICENSE BLOCK *****/

package heroku

import (
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(HerokuLogInputSpec)

	gs.MainGoTest(r, t)
}

func HerokuLogInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pConfig := NewPipelineConfig(nil)

	herokuLogInput := HerokuLogInput{}
	ith := new(plugins_ts.InputTestHelper)
	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockHelper.EXPECT().Hostname().Return("localhost")
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)

	startInput := func() {
		go func() {
			herokuLogInput.Run(ith.MockInputRunner, ith.MockHelper)
		}()
	}

	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())
	ith.PackSupply = make(chan *PipelinePack, 1)

	config := herokuLogInput.ConfigStruct().(*HerokuLogInputConfig)

	c.Specify("A HerokuLogInput", func() {
		ith.MockInputRunner.EXPECT().InChan().Return(ith.PackSupply)
		var deliverWg sync.WaitGroup
		deliverWg.Add(1)
		deliverCall := ith.MockInputRunner.EXPECT().Deliver(ith.Pack)
		deliverCall.Do(func(pack *PipelinePack) {
			deliverWg.Done()
		})

		startedChan := make(chan bool, 1)
		defer close(startedChan)
		ts := httptest.NewUnstartedServer(nil)

		herokuLogInput.starterFunc = func(hli *HerokuLogInput) error {
			ts.Start()
			startedChan <- true
			return nil
		}

		c.Specify("Check single log line", func() {
			err := herokuLogInput.Init(config)
			ts.Config = herokuLogInput.server
			c.Assume(err, gs.IsNil)

			startInput()
			ith.PackSupply <- ith.Pack
			<-startedChan

			req, err := http.NewRequest("POST", ts.URL+"/app-42",
				strings.NewReader("83 <158>1 2012-11-30T06:45:29+00:00 host app web.3 - State changed from starting to up\n"))
			req.Header.Add("Logplex-Msg-Count", "1")
			req.Header.Add("Content-Type", "application/logplex-1")
			req.Header.Add("Logplex-Frame-Id", "some-frame-id")
			req.Header.Add("Logplex-Drain-Token", "some-drain-token")
			req.Header.Set("User-Agent", "Logplex/v72")

			client := &http.Client{}
			resp, err := client.Do(req)

			c.Assume(err, gs.IsNil)
			resp.Body.Close()
			c.Assume(resp.StatusCode, gs.Equals, 200)

			deliverWg.Wait()

			c.Expect(ith.Pack.Message.GetTimestamp(), gs.Equals,
				int64(1354257929000000000))
			c.Expect(ith.Pack.Message.GetSeverity(), gs.Equals, int32(6))

			c.Expect(ith.Pack.Message.GetHostname(), gs.Equals, "app-42")
			c.Expect(ith.Pack.Message.GetLogger(), gs.Equals, "web.3")
			c.Expect(ith.Pack.Message.GetPayload(), gs.Equals,
				"State changed from starting to up")

			var (
				fieldValue interface{}
				ok         bool
			)

			fieldValue, ok = ith.Pack.Message.GetFieldValue("DrainToken")
			c.Assume(ok, gs.IsTrue)
			c.Expect(fieldValue, gs.Equals, "some-drain-token")
		})

		ts.Close()
		herokuLogInput.Stop()
	})
}

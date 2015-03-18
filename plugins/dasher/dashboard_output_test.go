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
#
# ***** END LICENSE BLOCK *****/

package dasher

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false

	r.AddSpec(DashboardOutputSpec)

	gs.MainGoTest(r, t)
}

func DashboardOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	pConfig := pipeline.NewPipelineConfig(nil)

	dashboardOutput := new(DashboardOutput)
	dashboardOutput.pConfig = pConfig

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	oth.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	oth.MockOutputRunner = pipelinemock.NewMockOutputRunner(ctrl)

	errChan := make(chan error, 1)

	startOutput := func() {
		go func() {
			errChan <- dashboardOutput.Run(oth.MockOutputRunner, oth.MockHelper)
		}()
	}

	if runtime.GOOS != "windows" {
		c.Specify("A DashboardOutput", func() {

			tmpdir, err := ioutil.TempDir("", "dashboard_output_test")
			c.Assume(err, gs.IsNil)
			config := dashboardOutput.ConfigStruct().(*DashboardOutputConfig)
			config.WorkingDirectory = tmpdir

			c.Specify("Init halts if basedirectory is not writable", func() {
				err := os.MkdirAll(tmpdir, 0400)
				c.Assume(err, gs.IsNil)
				defer os.RemoveAll(tmpdir)
				err = dashboardOutput.Init(config)
				c.Assume(err, gs.Not(gs.IsNil))
			})

			c.Specify("that is running", func() {
				startedChan := make(chan bool, 1)
				defer close(startedChan)
				ts := httptest.NewUnstartedServer(nil)

				dashboardOutput.starterFunc = func(hli *DashboardOutput) error {
					ts.Start()
					startedChan <- true
					return nil
				}

				ticker := make(chan time.Time)
				inChan := make(chan *pipeline.PipelinePack, 1)
				recycleChan := make(chan *pipeline.PipelinePack, 1)
				pack := pipeline.NewPipelinePack(recycleChan)
				pack.Message = pipeline_ts.GetTestMessage()
				pack.QueueCursor = "queuecursor"

				oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
				oth.MockOutputRunner.EXPECT().Ticker().Return(ticker)
				oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

				err := os.MkdirAll(tmpdir, 0700)
				c.Assume(err, gs.IsNil)
				defer os.RemoveAll(tmpdir)

				dashboardOutput.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Noop
				})

				c.Specify("sets custom http headers", func() {
					config.Headers = http.Header{
						"One":  []string{"two", "three"},
						"Four": []string{"five", "six", "seven"},
					}
					err = dashboardOutput.Init(config)
					c.Assume(err, gs.IsNil)
					ts.Config = dashboardOutput.server

					startOutput()

					inChan <- pack
					<-startedChan
					resp, err := http.Get(ts.URL)
					c.Assume(err, gs.IsNil)
					resp.Body.Close()
					c.Assume(resp.StatusCode, gs.Equals, 200)

					// Verify headers are there
					eq := reflect.DeepEqual(resp.Header["One"], config.Headers["One"])
					c.Expect(eq, gs.IsTrue)
					eq = reflect.DeepEqual(resp.Header["Four"], config.Headers["Four"])
					c.Expect(eq, gs.IsTrue)
				})

				close(inChan)
				c.Expect(<-errChan, gs.IsNil)

				ts.Close()
			})
		})
	}
}

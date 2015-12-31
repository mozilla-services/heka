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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package nagios

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false
	r.AddSpec(NagiosOutputSpec)
	gs.MainGoTest(r, t)
}

const echoFileTmpl = `#!/usr/bin/env sh
fname="%s"
echo "$1" "$2"> $fname
while read LINE; do
    echo ${LINE} >> $fname
done
`

func NagiosOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A NagiosOutput", func() {
		output := new(NagiosOutput)
		config := output.ConfigStruct().(*NagiosOutputConfig)
		config.Url = "http://localhost:55580/foo/bar"

		mockOutputRunner := pipelinemock.NewMockOutputRunner(ctrl)
		mockHelper := pipelinemock.NewMockPluginHelper(ctrl)

		inChan := make(chan *pipeline.PipelinePack)
		recycleChan := make(chan *pipeline.PipelinePack, 1)

		pack := pipeline.NewPipelinePack(recycleChan)
		msg := pipeline_ts.GetTestMessage()
		pack.Message = msg

		var req *http.Request
		var outputWg, reqWg sync.WaitGroup

		run := func() {
			mockOutputRunner.EXPECT().InChan().Return(inChan)
			mockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
			output.Run(mockOutputRunner, mockHelper)
			outputWg.Done()
		}

		const payload = "something"

		c.Specify("using HTTP transport", func() {
			// Spin up an HTTP listener.
			listener, err := net.Listen("tcp", "127.0.0.1:55580")
			c.Assume(err, gs.IsNil)

			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Connection", "close")
				r.ParseForm()
				req = r
				listener.Close()
				reqWg.Done()
			})

			go http.Serve(listener, mux)

			c.Specify("sends a valid HTTP POST", func() {
				err = output.Init(config)
				c.Assume(err, gs.IsNil)
				outputWg.Add(1)
				go run()

				msg.SetPayload("OK:" + payload)
				reqWg.Add(1)
				inChan <- pack
				close(inChan)
				outputWg.Wait()
				reqWg.Wait()

				c.Expect(req.FormValue("plugin_output"), gs.Equals, payload)
				c.Expect(req.FormValue("plugin_state"), gs.Equals, "0")
			})

			c.Specify("correctly maps alternate state", func() {
				err = output.Init(config)
				c.Assume(err, gs.IsNil)
				outputWg.Add(1)
				go run()

				msg.SetPayload("CRITICAL:" + payload)
				reqWg.Add(1)
				inChan <- pack
				close(inChan)
				outputWg.Wait()
				reqWg.Wait()

				c.Expect(req.FormValue("plugin_output"), gs.Equals, payload)
				c.Expect(req.FormValue("plugin_state"), gs.Equals, "2")
			})
		})

		if runtime.GOOS != "windows" {
			outPath := filepath.Join(os.TempDir(), "heka-nagios-test-output.txt")
			echoFile := fmt.Sprintf(echoFileTmpl, outPath)

			c.Specify("using a send_ncsa binary", func() {
				binPath := pipeline_ts.WriteStringToTmpFile(echoFile)
				os.Chmod(binPath, 0700)
				defer func() {
					os.Remove(binPath)
					os.Remove(outPath)
				}()
				config.SendNscaBin = binPath
				config.SendNscaArgs = []string{"arg1", "arg2"}
				err := output.Init(config)
				c.Assume(err, gs.IsNil)
				outputWg.Add(1)
				go run()

				c.Specify("sends args and the right data through", func() {
					msg.SetPayload("OK:" + payload)
					inChan <- pack
					close(inChan)
					outputWg.Wait()

					outFile, err := os.Open(outPath)
					c.Expect(err, gs.IsNil)
					reader := bufio.NewReader(outFile)
					line, _, err := reader.ReadLine()
					c.Expect(err, gs.IsNil)
					c.Expect(string(line), gs.Equals, strings.Join(config.SendNscaArgs, " "))
					line, _, err = reader.ReadLine()
					c.Expect(err, gs.IsNil)
					c.Expect(string(line), gs.Equals, "my.host.name GoSpec 0 "+payload)
				})

				c.Specify("correctly maps alternate state", func() {
					msg.SetPayload("WARNING:" + payload)
					inChan <- pack
					close(inChan)
					outputWg.Wait()

					outFile, err := os.Open(outPath)
					c.Expect(err, gs.IsNil)
					reader := bufio.NewReader(outFile)
					line, _, err := reader.ReadLine()
					c.Expect(err, gs.IsNil)
					c.Expect(string(line), gs.Equals, strings.Join(config.SendNscaArgs, " "))
					line, _, err = reader.ReadLine()
					c.Expect(err, gs.IsNil)
					c.Expect(string(line), gs.Equals, "my.host.name GoSpec 1 "+payload)
				})
			})
		}
	})
}

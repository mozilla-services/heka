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
#   Mike Trinkala (trink@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	ts "github.com/mozilla-services/heka/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net"
	"strings"
	"sync"
	"time"
)

type CarbonTestHelper struct {
	MockHelper       *MockPluginHelper
	MockOutputRunner *MockOutputRunner
}

func NewCarbonTestHelper(ctrl *gomock.Controller) (oth *CarbonTestHelper) {
	oth = new(CarbonTestHelper)
	oth.MockHelper = NewMockPluginHelper(ctrl)
	oth.MockOutputRunner = NewMockOutputRunner(ctrl)
	return
}

func CarbonOutputSpec(c gs.Context) {
	t := new(ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := NewCarbonTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A CarbonOutput", func() {
		carbonOutput := new(CarbonOutput)
		config := carbonOutput.ConfigStruct().(*CarbonOutputConfig)

		const count = 5
		lines := make([]string, count)
		baseTime := time.Now().UTC().Add(-10 * time.Second)
		nameTmpl := "stats.name.%d"

		for i := 0; i < count; i++ {
			statName := fmt.Sprintf(nameTmpl, i)
			statTime := baseTime.Add(time.Duration(i) * time.Second)
			lines[i] = fmt.Sprintf("%s %d %d", statName, i*2, statTime.Unix())
		}

		msg := getTestMessage()
		pack := NewPipelinePack(pConfig.inputRecycleChan)
		pack.Message = msg
		pack.Decoded = true

		c.Specify("writes out to the network", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
			inChanCall.Return(inChan)

			collectData := func(ch chan string) {
				ln, err := net.Listen("tcp", "localhost:2003")
				if err != nil {
					ch <- err.Error()
				}
				ch <- "ready"
				for i := 0; i < count; i++ {
					conn, err := ln.Accept()
					if err != nil {
						ch <- err.Error()
					}
					b := make([]byte, 1000)
					n, _ := conn.Read(b)
					ch <- string(b[0:n])
				}
			}
			ch := make(chan string, count) // don't block on put
			go collectData(ch)
			result := <-ch // wait for server

			err := carbonOutput.Init(config)
			c.Assume(err, gs.IsNil)

			pack.Message.SetPayload(strings.Join(lines, "\n"))

			go func() {
				wg.Add(1)
				carbonOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack

			close(inChan)
			wg.Wait() // wait for close to finish, prevents intermittent test failures

			matchBytes := make([]byte, 0, 1000)
			err = createProtobufStream(pack, &matchBytes)
			c.Expect(err, gs.IsNil)

			result = <-ch
			computed_result := strings.Join(lines, "\n") + "\n"
			c.Expect(result, gs.Equals, computed_result)
		})
	})

}

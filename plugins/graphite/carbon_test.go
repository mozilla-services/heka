/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#   Mike Trinkala (trink@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package graphite

import (
	"code.google.com/p/gomock/gomock"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	. "github.com/mozilla-services/heka/pipelinemock"
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

type collectFunc func(chPort chan<- int, chData chan<- string, chError chan<- error)

func CarbonOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := NewCarbonTestHelper(ctrl)
	var wg sync.WaitGroup
	pConfig := NewPipelineConfig(nil)

	// make test data
	const count = 5
	lines := make([]string, count)
	baseTime := time.Now().UTC().Add(-10 * time.Second)
	for i := 0; i < count; i++ {
		statName := fmt.Sprintf("stats.name.%d", i)
		statTime := baseTime.Add(time.Duration(i) * time.Second)
		lines[i] = fmt.Sprintf("%s %d %d", statName, i*2, statTime.Unix())
	}
	submit_data := strings.Join(lines, "\n")
	expected_data := submit_data + "\n"

	// a helper to make new test packs
	newpack := func() *PipelinePack {
		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true
		pack.Message.SetPayload(submit_data)
		return pack
	}

	// collectDataTCP and collectDataUDP functions; when ready reports its port on
	// chPort, or error on chErr; when data is received it is reported on chData
	collectDataTCP := func(chPort chan<- int, chData chan<- string, chError chan<- error) {
		tcpaddr, err := net.ResolveTCPAddr("tcp", ":0")
		if err != nil {
			chError <- err
			return
		}

		listener, err := net.ListenTCP("tcp", tcpaddr)
		if err != nil {
			chError <- err
			return
		}
		chPort <- listener.Addr().(*net.TCPAddr).Port

		conn, err := listener.Accept()
		if err != nil {
			chError <- err
			return
		}

		b := make([]byte, 10000)
		n, err := conn.Read(b)
		if err != nil {
			chError <- err
			return
		}

		chData <- string(b[0:n])
	}

	collectDataUDP := func(chPort chan<- int, chData chan<- string, chError chan<- error) {
		udpaddr, err := net.ResolveUDPAddr("udp", ":0")
		if err != nil {
			chError <- err
			return
		}

		conn, err := net.ListenUDP("udp", udpaddr)
		if err != nil {
			chError <- err
			return
		}
		chPort <- conn.LocalAddr().(*net.UDPAddr).Port

		b := make([]byte, 10000)
		n, _, err := conn.ReadFromUDP(b)
		if err != nil {
			chError <- err
			return
		}
		chData <- string(b[0:n])
	}

	doit := func(protocol string, collectData collectFunc) {
		c.Specify("A CarbonOutput ", func() {
			inChan := make(chan *PipelinePack, 1)
			carbonOutput := new(CarbonOutput)
			config := carbonOutput.ConfigStruct().(*CarbonOutputConfig)
			pack := newpack()

			c.Specify("writes "+protocol+" to the network", func() {
				inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
				inChanCall.Return(inChan)

				chError := make(chan error, count)
				chPort := make(chan int, count)
				chData := make(chan string, count)
				go collectData(chPort, chData, chError)

			WAIT_FOR_DATA:
				for {
					select {
					case port := <-chPort:
						// data collection server is ready, start CarbonOutput
						config.Address = fmt.Sprintf("127.0.0.1:%d", port)
						config.Protocol = protocol
						err := carbonOutput.Init(config)
						c.Assume(err, gs.IsNil)
						go func() {
							wg.Add(1)
							carbonOutput.Run(oth.MockOutputRunner, oth.MockHelper)
							wg.Done()
						}()

						// Send the pack.
						inChan <- pack

					case data := <-chData:
						close(inChan)
						wg.Wait() // wait for close to finish, prevents intermittent test failures
						c.Expect(data, gs.Equals, expected_data)
						break WAIT_FOR_DATA

					case err := <-chError:
						// fail
						c.Assume(err, gs.IsNil)
						return
					}
				}
			})
		})
	}

	doit("tcp", collectDataTCP)
	doit("udp", collectDataUDP)
}

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
#   Mike Trinkala (trink@mozilla.com)
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package graphite

import (
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	. "github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
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
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := NewCarbonTestHelper(ctrl)
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
		pack.Message.SetPayload(submit_data)
		pack.QueueCursor = "queuecursor"
		return pack
	}

	c.Specify("A CarbonOutput ", func() {
		inChan := make(chan *PipelinePack, 1)
		output := new(CarbonOutput)
		config := output.ConfigStruct().(*CarbonOutputConfig)
		pack := newpack()

		errChan := make(chan error, 1)
		connChan := make(chan net.Conn, 1)
		dataChan := make(chan string, count)

		startOutput := func(output *CarbonOutput, oth *CarbonTestHelper) {
			errChan <- output.Run(oth.MockOutputRunner, oth.MockHelper)
		}

		// collectData waits for data to come in on the provided connection.
		// It "returns" data using channels, either with an error on the
		// errChan or data on the dataChan.
		collectData := func(conn net.Conn) {
			b := make([]byte, 10000)
			n, err := conn.Read(b)
			if err != nil {
				errChan <- err
				return
			}
			dataChan <- string(b[0:n])
		}

		inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
		inChanCall.Return(inChan)

		var (
			conn net.Conn
			err  error
		)

		oth.MockOutputRunner.EXPECT().UpdateCursor(pack.QueueCursor)

		c.Specify("using TCP", func() {
			var listener net.Listener

			config.Protocol = "tcp"
			listenerChan := make(chan net.Listener, 1)

			// startListener starts a TCP server waiting for data. It
			// "returns" data over channels. First it will *either* return an
			// error on the errChan *or* the listener on the listenerChan. If
			// listening succeeds it will wait for a connection attempt,
			// either returning an error on the errChan or the connection on
			// the connChan.
			startListener := func() {
				tcpaddr, err := net.ResolveTCPAddr("tcp", ":0")
				if err != nil {
					errChan <- err
					return
				}

				listener, err := net.ListenTCP("tcp", tcpaddr)
				if err != nil {
					errChan <- err
					return
				}
				listenerChan <- listener

				conn, err := listener.Accept()
				if err != nil {
					errChan <- err
					return
				}
				connChan <- conn
			}

			go startListener()
			select {
			case err = <-errChan:
				// If we get an error herer it won't be nil, but we use
				// c.Assume to trigger the test failure.
				c.Assume(err, gs.IsNil)
			case listener = <-listenerChan:
				defer listener.Close()
			}

			c.Specify("writes to the network", func() {

				config.Address = fmt.Sprintf("127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
				err = output.Init(config)
				c.Assume(err, gs.IsNil)
				inChan <- pack
				go startOutput(output, oth)

				select {
				case err = <-errChan:
					// If we get an error herer it won't be nil, but we use
					// c.Assume to trigger the test failure.
					c.Assume(err, gs.IsNil)
				case conn = <-connChan:
					defer conn.Close()
				}

				go collectData(conn)

				select {
				case err = <-errChan:
					// If we get an error herer it won't be nil, but we use
					// c.Assume to trigger the test failure.
					c.Assume(err, gs.IsNil)
				case data := <-dataChan:
					c.Expect(data, gs.Equals, expected_data)
				}

				close(inChan)
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})

		c.Specify("using UDP", func() {
			config.Protocol = "udp"

			// startListener starts a UDP server waiting for data. It
			// "returns" data over channels. It will either return a net.Conn
			// on the connChan or an error on the errChan.
			startListener := func() {
				udpAddr, err := net.ResolveUDPAddr("udp", ":0")
				if err != nil {
					errChan <- err
					return
				}
				conn, err := net.ListenUDP("udp", udpAddr)
				if err != nil {
					errChan <- err
					return
				}
				connChan <- conn
			}

			go startListener()
			select {
			case err = <-errChan:
				c.Assume(err, gs.IsNil)
			case conn = <-connChan:
				defer conn.Close()
			}

			c.Specify("writes to the network", func() {
				config.Address = fmt.Sprintf("127.0.0.1:%d", conn.LocalAddr().(*net.UDPAddr).Port)
				err = output.Init(config)
				c.Assume(err, gs.IsNil)
				inChan <- pack

				go startOutput(output, oth)
				go collectData(conn)

				select {
				case err = <-errChan:
					// If we get an error herer it won't be nil, but we use
					// c.Assume to trigger the test failure.
					c.Assume(err, gs.IsNil)
				case data := <-dataChan:
					c.Expect(data, gs.Equals, expected_data)
				}

				close(inChan)
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})

			c.Specify("splits packets that are too long", func() {
				config.Address = fmt.Sprintf("127.0.0.1:%d", conn.LocalAddr().(*net.UDPAddr).Port)
				err = output.Init(config)
				c.Assume(err, gs.IsNil)
				output.bufSplitSize = 1 // Forces a separate packet for each stat.

				inChan <- pack

				go startOutput(output, oth)

				for i := 0; i < count; i++ {
					go collectData(conn)

					select {
					case err = <-errChan:
						// If we get an error herer it won't be nil, but we use
						// c.Assume to trigger the test failure.
						c.Assume(err, gs.IsNil)
					case data := <-dataChan:
						c.Expect(data, gs.Equals, lines[i]+"\n")
					}
				}

				close(inChan)
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})
	})
}

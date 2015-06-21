/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package udp

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/plugins"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func UdpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)

	udpOutput := new(UdpOutput)
	config := udpOutput.ConfigStruct().(*UdpOutputConfig)

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	encoder := new(plugins.PayloadEncoder)
	encoder.Init(new(plugins.PayloadEncoderConfig))

	inChan := make(chan *pipeline.PipelinePack, 1)
	rChan := make(chan *pipeline.PipelinePack, 1)
	var wg sync.WaitGroup
	var rAddr net.Addr

	c.Specify("A UdpOutput", func() {
		msg := pipeline_ts.GetTestMessage()
		payload := "Write me out to the network."
		msg.SetPayload(payload)
		pack := pipeline.NewPipelinePack(rChan)
		pack.Message = msg

		oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
		oth.MockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
		oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
		oth.MockOutputRunner.EXPECT().Encode(pack).Return(encoder.Encode(pack))

		c.Specify("using UDP", func() {
			addr := "127.0.0.1:45678"
			config.Address = addr
			ch := make(chan string, 1)

			collectData := func() {
				conn, err := net.ListenPacket("udp", addr)
				if err != nil {
					ch <- err.Error()
					return
				}

				ch <- "ready"
				b := make([]byte, 1000)
				var n int
				n, rAddr, _ = conn.ReadFrom(b)
				ch <- string(b[:n])
				conn.Close()
			}

			go collectData()
			result := <-ch // Wait for server to be ready.
			c.Assume(result, gs.Equals, "ready")

			c.Specify("writes out to the network", func() {
				err := udpOutput.Init(config)
				c.Assume(err, gs.IsNil)

				wg.Add(1)
				go func() {
					err = udpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()

				inChan <- pack
				result = <-ch

				c.Expect(result, gs.Equals, payload)
				close(inChan)
				wg.Wait()
			})

			c.Specify("uses the specified local address", func() {
				config.LocalAddress = "localhost:12345"
				err := udpOutput.Init(config)
				c.Assume(err, gs.IsNil)

				wg.Add(1)
				go func() {
					err = udpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()

				inChan <- pack
				result = <-ch

				c.Expect(result, gs.Equals, payload)
				c.Expect(rAddr.Network(), gs.Equals, "udp")
				c.Expect(rAddr.String(), gs.Equals, "127.0.0.1:12345")
				close(inChan)
				wg.Wait()
			})
		})

		c.Specify("using Unix datagrams", func() {
			if runtime.GOOS == "windows" {
				return
			}

			testUnixAddr := func() string {
				f, err := ioutil.TempFile("", "_heka_test_sock")
				c.Assume(err, gs.IsNil)
				addr := f.Name()
				f.Close()
				os.Remove(addr)
				return addr
			}

			config.Address = testUnixAddr()
			config.Net = "unixgram"
			ch := make(chan string, 1)
			var wg sync.WaitGroup
			var rAddr net.Addr

			collectData := func() {
				conn, err := net.ListenPacket("unixgram", config.Address)
				if err != nil {
					ch <- err.Error()
					return
				}

				ch <- "ready"
				b := make([]byte, 1000)
				var n int
				n, rAddr, _ = conn.ReadFrom(b)
				ch <- string(b[:n])
				conn.Close()
				err = os.Remove(config.Address)
				var errMsg string
				if err != nil {
					errMsg = err.Error()
				}
				ch <- errMsg
			}

			go collectData()
			result := <-ch // Wait for server to be ready.
			c.Assume(result, gs.Equals, "ready")

			c.Specify("writes out to the network", func() {
				err := udpOutput.Init(config)
				c.Assume(err, gs.IsNil)

				wg.Add(1)
				go func() {
					err = udpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()

				inChan <- pack
				result = <-ch

				c.Expect(result, gs.Equals, payload)
				close(inChan)
				wg.Wait()
				result = <-ch
				c.Expect(result, gs.Equals, "")
			})

			c.Specify("uses the specified local address", func() {
				config.LocalAddress = testUnixAddr()
				err := udpOutput.Init(config)
				c.Assume(err, gs.IsNil)

				wg.Add(1)
				go func() {
					err = udpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
					c.Expect(err, gs.IsNil)
					wg.Done()
				}()

				inChan <- pack
				result = <-ch

				c.Expect(result, gs.Equals, payload)
				c.Expect(rAddr.Network(), gs.Equals, "unixgram")
				c.Expect(rAddr.String(), gs.Equals, config.LocalAddress)
				close(inChan)
				wg.Wait()
			})

		})
	})
	c.Specify("drop message contents if their size is bigger than allowed UDP datagram size", func() {
		huge_msg := pipeline_ts.GetTestMessage()
		payload := strings.Repeat("2", 131014)

		huge_msg.SetPayload(payload)

		huge_pack := pipeline.NewPipelinePack(rChan)
		huge_pack.Message = huge_msg

		oth.MockOutputRunner.EXPECT().InChan().Return(inChan)
		oth.MockOutputRunner.EXPECT().UpdateCursor("").AnyTimes()
		oth.MockOutputRunner.EXPECT().Encoder().Return(encoder)
		oth.MockOutputRunner.EXPECT().Encode(huge_pack).Return(encoder.Encode(huge_pack))
		oth.MockOutputRunner.EXPECT().LogError(fmt.Errorf("Message has exceeded allowed UDP data size: 131014 > 65507"))

		config.Address = "localhost:12345"
		err := udpOutput.Init(config)

		c.Assume(err, gs.IsNil)
		wg.Add(1)
		go func() {
			err = udpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
			c.Expect(err, gs.IsNil)
			wg.Done()
		}()
		inChan <- huge_pack

		close(inChan)
		wg.Wait()
	})

	c.Specify("checks validation of of Maximum message size limit", func() {

		config.Address = "localhost:12345"
		config.MaxMessageSize = 100

		err := udpOutput.Init(config)

		c.Assume(err.Error(), gs.Equals, "Maximum message size can't be smaller than 512 bytes.")
	})
}

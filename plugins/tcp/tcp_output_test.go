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
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"bytes"
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"net"
	"sync"
)

func TcpOutputSpec(c gs.Context) {
	t := new(pipeline_ts.SimpleT)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oth := plugins_ts.NewOutputTestHelper(ctrl)
	var wg sync.WaitGroup
	inChan := make(chan *PipelinePack, 1)
	pConfig := NewPipelineConfig(nil)

	c.Specify("A TcpOutput", func() {
		tcpOutput := new(TcpOutput)
		config := tcpOutput.ConfigStruct().(*TcpOutputConfig)
		tcpOutput.connection = pipeline_ts.NewMockConn(ctrl)

		msg := pipeline_ts.GetTestMessage()
		pack := NewPipelinePack(pConfig.InputRecycleChan())
		pack.Message = msg
		pack.Decoded = true

		c.Specify("correctly formats protocol buffer stream output", func() {
			outBytes := make([]byte, 0, 200)
			err := ProtobufEncodeMessage(pack, &outBytes)
			c.Expect(err, gs.IsNil)

			b := []byte{30, 2, 8, uint8(proto.Size(pack.Message)), 31, 10, 16} // sanity check the header and the start of the protocol buffer
			c.Expect(bytes.Equal(b, (outBytes)[:len(b)]), gs.IsTrue)
		})

		c.Specify("writes out to the network", func() {
			inChanCall := oth.MockOutputRunner.EXPECT().InChan().AnyTimes()
			inChanCall.Return(inChan)

			collectData := func(ch chan string) {
				ln, err := net.Listen("tcp", "localhost:9125")
				if err != nil {
					ch <- err.Error()
				}
				ch <- "ready"
				conn, err := ln.Accept()
				if err != nil {
					ch <- err.Error()
				}
				b := make([]byte, 1000)
				n, _ := conn.Read(b)
				ch <- string(b[0:n])
			}
			ch := make(chan string, 1) // don't block on put
			go collectData(ch)
			result := <-ch // wait for server

			err := tcpOutput.Init(config)
			c.Assume(err, gs.IsNil)

			outStr := "Write me out to the network"
			pack.Message.SetPayload(outStr)
			go func() {
				wg.Add(1)
				tcpOutput.Run(oth.MockOutputRunner, oth.MockHelper)
				wg.Done()
			}()
			inChan <- pack

			close(inChan)
			wg.Wait() // wait for close to finish, prevents intermittent test failures

			matchBytes := make([]byte, 0, 1000)
			err = ProtobufEncodeMessage(pack, &matchBytes)
			c.Expect(err, gs.IsNil)

			result = <-ch
			c.Expect(result, gs.Equals, string(matchBytes))
		})
	})
}

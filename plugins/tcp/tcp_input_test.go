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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	plugins_ts "github.com/mozilla-services/heka/plugins/testsupport"
	"github.com/rafrombrc/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

type address struct {
	network string
	str     string
}

func (a *address) Network() string {
	return a.network
}

func (a *address) String() string {
	return a.str
}

func TcpInputSpec(c gs.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pConfig := NewPipelineConfig(nil)
	ith := new(plugins_ts.InputTestHelper)
	ith.Msg = pipeline_ts.GetTestMessage()
	ith.Pack = NewPipelinePack(pConfig.InputRecycleChan())

	ith.AddrStr = "localhost:55565"
	ith.ResolvedAddrStr = "127.0.0.1:55565"

	ith.MockHelper = pipelinemock.NewMockPluginHelper(ctrl)
	ith.MockInputRunner = pipelinemock.NewMockInputRunner(ctrl)
	ith.MockDeliverer = pipelinemock.NewMockDeliverer(ctrl)
	ith.MockSplitterRunner = pipelinemock.NewMockSplitterRunner(ctrl)
	ith.PackSupply = make(chan *PipelinePack, 1)

	c.Specify("A TcpInput", func() {
		tcpInput := &TcpInput{}
		config := &TcpInputConfig{
			Net:     "tcp",
			Address: ith.AddrStr,
		}

		bytesChan := make(chan []byte, 1)
		errChan := make(chan error, 1)

		startServer := func() {
			ith.MockInputRunner.EXPECT().Name().Return("mock_name")
			ith.MockInputRunner.EXPECT().NewDeliverer(gomock.Any()).Return(ith.MockDeliverer)
			ith.MockDeliverer.EXPECT().Done()
			ith.MockInputRunner.EXPECT().NewSplitterRunner(gomock.Any()).Return(
				ith.MockSplitterRunner)
			ith.MockSplitterRunner.EXPECT().UseMsgBytes().Return(false)
			ith.MockSplitterRunner.EXPECT().SetPackDecorator(gomock.Any())
			ith.MockSplitterRunner.EXPECT().Done()

			// splitCall gets called twice. The first time it returns nil, the
			// second time it returns io.EOF.
			splitCall := ith.MockSplitterRunner.EXPECT().SplitStream(gomock.Any(),
				ith.MockDeliverer).AnyTimes()
			splitCall.Do(func(conn net.Conn, del Deliverer) {
				recd, _ := ioutil.ReadAll(conn)
				bytesChan <- recd
				splitCall.Return(io.EOF)
			})

			err := tcpInput.Run(ith.MockInputRunner, ith.MockHelper)
			errChan <- err
		}

		c.Specify("not using TLS", func() {
			err := tcpInput.Init(config)
			c.Assume(err, gs.IsNil)
			c.Expect(tcpInput.listener.Addr().String(), gs.Equals, ith.ResolvedAddrStr)

			c.Specify("accepts connections and passes them to the splitter", func() {
				go startServer()
				data := []byte("THIS IS THE DATA")

				outConn, err := net.Dial("tcp", ith.AddrStr)
				c.Assume(err, gs.IsNil)
				_, err = outConn.Write(data)
				c.Expect(err, gs.IsNil)
				outConn.Close()

				recd := <-bytesChan

				c.Expect(err, gs.IsNil)
				c.Expect(string(recd), gs.Equals, string(data))

				tcpInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})

		c.Specify("using TLS", func() {
			config.UseTls = true

			c.Specify("fails to init w/ missing key or cert file", func() {
				config.Tls = TlsConfig{}
				err := tcpInput.Init(config)
				c.Expect(err, gs.Not(gs.IsNil))
				c.Expect(err.Error(), gs.Equals,
					"TLS config requires both cert_file and key_file value.")
			})

			config.Tls = TlsConfig{
				CertFile: "./testsupport/cert.pem",
				KeyFile:  "./testsupport/key.pem",
			}

			c.Specify("accepts connections and passes them to the splitter", func() {
				err := tcpInput.Init(config)
				c.Expect(err, gs.IsNil)

				go startServer()
				data := []byte("This is a test.")

				clientConfig := new(tls.Config)
				clientConfig.InsecureSkipVerify = true
				outConn, err := tls.Dial("tcp", ith.AddrStr, clientConfig)
				c.Assume(err, gs.IsNil)
				outConn.SetWriteDeadline(time.Now().Add(time.Duration(10000)))
				n, err := outConn.Write(data)
				c.Expect(err, gs.IsNil)
				c.Expect(n, gs.Equals, len(data))
				outConn.Close()

				recd := <-bytesChan

				c.Expect(err, gs.IsNil)
				c.Expect(string(recd), gs.Equals, string(data))

				tcpInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})

			c.Specify("doesn't accept connections below specified min TLS version", func() {
				config.Tls.MinVersion = "TLS12"
				err := tcpInput.Init(config)
				c.Expect(err, gs.IsNil)

				go startServer()

				clientConfig := &tls.Config{
					InsecureSkipVerify: true,
					MaxVersion:         tls.VersionTLS11,
				}
				conn, err := tls.Dial("tcp", ith.AddrStr, clientConfig)
				c.Expect(conn, gs.IsNil)
				c.Expect(err, gs.Not(gs.IsNil))

				<-bytesChan

				tcpInput.Stop()
				err = <-errChan
				c.Expect(err, gs.IsNil)
			})
		})
	})
}

func TcpInputSpecFailure(c gs.Context) {
	tcpInput := TcpInput{}
	err := tcpInput.Init(&TcpInputConfig{
		Net:     "udp",
		Address: "localhost:55565",
	})
	c.Assume(err, gs.Not(gs.IsNil))
	c.Assume(err.Error(), gs.Equals, "ResolveTCPAddress failed: unknown network udp\n")

}

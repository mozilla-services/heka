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
	"crypto/tls"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"net"
	"regexp"
	"sync"
	"sync/atomic"
)

// Output plugin that sends messages via TCP using the Heka protocol.
type TcpOutput struct {
	conf                *TcpOutputConfig
	address             string
	localAddress        net.Addr
	connection          net.Conn
	processMessageCount int64
	name                string
	reportLock          sync.Mutex
	bufferedOut         *BufferedOutput
}

// ConfigStruct for TcpOutput plugin.
type TcpOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address      string
	LocalAddress string `toml:"local_address"`
	UseTls       bool   `toml:"use_tls"`
	Tls          TlsConfig
	// Interval at which the output queue logs will roll, in
	// seconds. Defaults to 300.
	TickerInterval uint `toml:"ticker_interval"`
}

func (t *TcpOutput) ConfigStruct() interface{} {
	return &TcpOutputConfig{Address: "localhost:9125",
		TickerInterval: uint(300)}
}

func (t *TcpOutput) SetName(name string) {
	re := regexp.MustCompile("\\W")
	t.name = re.ReplaceAllString(name, "_")
}

func (t *TcpOutput) Init(config interface{}) (err error) {
	t.conf = config.(*TcpOutputConfig)
	t.address = t.conf.Address

	if t.conf.LocalAddress != "" {
		// Error out if use_tls and local_address options are both set for now
		if t.conf.UseTls {
			return fmt.Errorf("Cannot combine local_address %s and use_tls config options", t.localAddress)
		}
		t.localAddress, err = net.ResolveTCPAddr("tcp", t.conf.LocalAddress)
	}

	if t.bufferedOut, err = NewBufferedOutput("output_queue", t.name); err != nil {
		return
	}
	return
}

func (t *TcpOutput) connect() (err error) {
	dialer := &net.Dialer{LocalAddr: t.localAddress}

	if t.conf.UseTls {
		var goTlsConf *tls.Config
		if goTlsConf, err = CreateGoTlsConfig(&t.conf.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
		// We should use DialWithDialer but its not in GOLANG release yet.
		// https://code.google.com/p/go/source/detail?r=3d37606fb79393f22a69573afe31f0b0cd4866e3&name=default
		// t.connection, err = tls.DialWithDialer(dialer, "tcp", t.address, goTlsConf)
		t.connection, err = tls.Dial("tcp", t.address, goTlsConf)
	} else {
		t.connection, err = dialer.Dial("tcp", t.address)
	}
	return
}

func (t *TcpOutput) SendRecord(record []byte) (err error) {
	var n int
	if t.connection == nil {
		if err = t.connect(); err != nil {
			// Explicitly set t.connection to nil b/c Go, see
			// http://golang.org/doc/faq#nil_error.
			t.connection = nil
			return
		}
	}

	cleanupConn := func() {
		if t.connection != nil {
			t.connection.Close()
			t.connection = nil
		}
	}

	if n, err = t.connection.Write(record); err != nil {
		cleanupConn()
		err = fmt.Errorf("writing to %s: %s", t.address, err)
	} else if n != len(record) {
		cleanupConn()
		err = fmt.Errorf("truncated output to: %s", t.address)
	}

	return
}

func (t *TcpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var (
		ok          = true
		pack        *PipelinePack
		inChan      = or.InChan()
		ticker      = or.Ticker()
		outputExit  = make(chan error)
		outputError = make(chan error, 5)
		stopChan    = make(chan bool, 1)
	)

	defer func() {
		if t.connection != nil {
			t.connection.Close()
			t.connection = nil
		}
	}()

	t.bufferedOut.Output(t, outputError, outputExit, stopChan)

	for ok {
		select {
		case e := <-outputError:
			or.LogError(e)

		case pack, ok = <-inChan:
			if !ok {
				stopChan <- true
				<-outputExit
				break
			}
			atomic.AddInt64(&t.processMessageCount, 1)
			if err = t.bufferedOut.QueueRecord(pack); err != nil {
				or.LogError(err)
			}
			pack.Recycle()

		case <-ticker:
			if err = t.bufferedOut.RollQueue(); err != nil {
				return
			}

		case err = <-outputExit:
			ok = false
		}
	}

	return
}

func init() {
	RegisterPlugin("TcpOutput", func() interface{} {
		return new(TcpOutput)
	})
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide plugin state
// information to the Heka report and dashboard.
func (t *TcpOutput) ReportMsg(msg *message.Message) error {
	t.reportLock.Lock()
	defer t.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount", atomic.LoadInt64(&t.processMessageCount), "count")
	t.bufferedOut.ReportMsg(msg)
	return nil
}

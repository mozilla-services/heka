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
#   Carlos Diaz-Padron (cpadron@mozilla.com,carlos@carlosdp.io)
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"crypto/tls"
	"fmt"
	"net"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
)

// Output plugin that sends messages via TCP using the Heka protocol.
type TcpOutput struct {
	processMessageCount int64
	dropMessageCount    int64
	keepAliveDuration   time.Duration
	conf                *TcpOutputConfig
	address             string
	localAddress        net.Addr
	connection          net.Conn
	name                string
	reportLock          sync.Mutex
	or                  OutputRunner
	pConfig             *PipelineConfig
}

// ConfigStruct for TcpOutput plugin.
type TcpOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address      string
	LocalAddress string `toml:"local_address"`
	UseTls       bool   `toml:"use_tls"`
	Tls          TlsConfig
	// Interval at which the output queue logs will roll, in seconds. Defaults
	// to 300.
	TickerInterval uint `toml:"ticker_interval"`
	// Allows for a default encoder.
	Encoder string
	// Set to true if TCP Keep Alive should be used.
	KeepAlive bool `toml:"keep_alive"`
	// Integer indicating seconds between keep alives.
	KeepAlivePeriod int `toml:"keep_alive_period"`
	// Number of successfully processed messages to re-establish the TCP
	// connection after.  Defaults to 0 (never)
	ReconnectAfter int64 `toml:"reconnect_after"`
	// Specifies whether or not Heka's stream framing wil be applied to the
	// output. We do some magic to default to true if ProtobufEncoder is used,
	// false otherwise.
	UseFraming *bool `toml:"use_framing"`
	// Defaults to true for TcpOutput.
	UseBuffering *bool `toml:"use_buffering"`
	Buffering    QueueBufferConfig
}

func (t *TcpOutput) ConfigStruct() interface{} {
	b := true
	queueConfig := QueueBufferConfig{
		CursorUpdateCount: 50,
		MaxBufferSize:     0,
		MaxFileSize:       128 * 1024 * 1024,
		FullAction:        "shutdown",
	}
	return &TcpOutputConfig{
		Address:      "localhost:9125",
		Encoder:      "ProtobufEncoder",
		UseBuffering: &b,
		Buffering:    queueConfig,
	}
}

func (t *TcpOutput) SetName(name string) {
	re := regexp.MustCompile("\\W")
	t.name = re.ReplaceAllString(name, "_")
}

func (t *TcpOutput) Init(config interface{}) (err error) {
	t.conf = config.(*TcpOutputConfig)
	t.address = t.conf.Address

	if t.conf.LocalAddress != "" {
		// Error out if use_tls and local_address options are both set for now.
		if t.conf.UseTls {
			return fmt.Errorf("Cannot combine local_address %s and use_tls config options",
				t.localAddress)
		}
		t.localAddress, err = net.ResolveTCPAddr("tcp", t.conf.LocalAddress)
	}

	if t.conf.KeepAlivePeriod != 0 {
		t.keepAliveDuration = time.Duration(t.conf.KeepAlivePeriod) * time.Second
	}

	return
}

func (t *TcpOutput) Prepare(or OutputRunner, h PluginHelper) (err error) {
	if t.conf.UseFraming == nil {
		// Nothing was specified, we'll default to framing IFF ProtobufEncoder
		// is being used.
		if _, ok := or.Encoder().(*ProtobufEncoder); ok {
			or.SetUseFraming(true)
		}
	}

	t.pConfig = h.PipelineConfig()
	t.or = or

	return nil
}

func (t *TcpOutput) cleanupConn() {
	if t.connection != nil {
		t.connection.Close()
		t.connection = nil
	}
}

func (t *TcpOutput) CleanUp() {
	t.cleanupConn()
}

func (t *TcpOutput) ProcessMessage(pack *PipelinePack) (err error) {
	if t.connection == nil {
		if err = t.connect(); err != nil {
			// Explicitly set t.connection to nil because Go, see
			// http://golang.org/doc/faq#nil_error.
			t.connection = nil
			return NewRetryMessageError("can't connect: %s", err)
		}
	}

	var (
		n      int
		record []byte
	)

	if record, err = t.or.Encode(pack); err != nil {
		atomic.AddInt64(&t.dropMessageCount, 1)
		return fmt.Errorf("can't encode: %s", err)
	}

	if n, err = t.connection.Write(record); err != nil {
		t.cleanupConn()
		err = NewRetryMessageError("writing to %s: %s", t.address, err)
	} else if n != len(record) {
		t.cleanupConn()
		err = NewRetryMessageError("truncated output to: %s", t.address)
	} else {
		atomic.AddInt64(&t.processMessageCount, 1)
		t.or.UpdateCursor(pack.QueueCursor)
		if t.conf.ReconnectAfter > 0 && atomic.LoadInt64(&t.processMessageCount)%t.conf.ReconnectAfter == 0 {
			t.cleanupConn()
		}
	}

	return err
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
	if err == nil && t.conf.KeepAlive {
		tcpConn, ok := t.connection.(*net.TCPConn)
		if !ok {
			t.or.LogError(fmt.Errorf("KeepAlive only supported for TCP Connections."))
		} else {
			tcpConn.SetKeepAlive(t.conf.KeepAlive)
			if t.keepAliveDuration != 0 {
				tcpConn.SetKeepAlivePeriod(t.keepAliveDuration)
			}
		}
	}
	return
}

// Satisfies the `pipeline.ReportingPlugin` interface to provide plugin state
// information to the Heka report and dashboard.
func (t *TcpOutput) ReportMsg(msg *message.Message) error {
	t.reportLock.Lock()
	defer t.reportLock.Unlock()

	message.NewInt64Field(msg, "ProcessMessageCount",
		atomic.LoadInt64(&t.processMessageCount), "count")
	message.NewInt64Field(msg, "DropMessageCount",
		atomic.LoadInt64(&t.dropMessageCount), "count")

	return nil
}

func init() {
	RegisterPlugin("TcpOutput", func() interface{} {
		return new(TcpOutput)
	})
}

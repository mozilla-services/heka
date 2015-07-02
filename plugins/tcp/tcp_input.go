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
#   Carlos Diaz-Padron (cpadron@mozilla.com,carlos@carlosdp.io)
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	. "github.com/mozilla-services/heka/pipeline"
)

// Input plugin implementation that listens for Heka protocol messages on a
// specified TCP socket. Creates a separate goroutine for each TCP connection.
type TcpInput struct {
	keepAliveDuration time.Duration
	listener          net.Listener
	wg                sync.WaitGroup
	stopChan          chan bool
	ir                InputRunner
	config            *TcpInputConfig
}

type TcpInputConfig struct {
	// Network type (e.g. "tcp", "tcp4", "tcp6", "unix" or "unixpacket").
	// Needs to match the input type.
	Net string
	// String representation of the address of the network connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
	// Set to true if the TCP connection should be tunneled through TLS.
	// Requires additional Tls config section.
	UseTls bool `toml:"use_tls"`
	// Subsection for TLS configuration.
	Tls TlsConfig
	// Set to true if TCP Keep Alive should be used.
	KeepAlive bool `toml:"keep_alive"`
	// Integer indicating seconds between keep alives.
	KeepAlivePeriod int `toml:"keep_alive_period"`
	// So we can default to using ProtobufDecoder.
	Decoder string
	// So we can default to using HekaFramingSplitter.
	Splitter string
}

func (t *TcpInput) ConfigStruct() interface{} {
	config := &TcpInputConfig{
		Net:      "tcp",
		Decoder:  "ProtobufDecoder",
		Splitter: "HekaFramingSplitter",
	}
	config.Tls = TlsConfig{PreferServerCiphers: true}
	return config
}

func (t *TcpInput) Init(config interface{}) error {
	var err error
	t.config = config.(*TcpInputConfig)
	address, err := net.ResolveTCPAddr(t.config.Net, t.config.Address)
	if err != nil {
		return fmt.Errorf("ResolveTCPAddress failed: %s\n", err.Error())
	}
	t.listener, err = net.ListenTCP(t.config.Net, address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	// We're already listening, make sure we clean up if init fails later on.
	closeIt := true
	defer func() {
		if closeIt {
			t.listener.Close()
		}
	}()
	if t.config.UseTls {
		if err = t.setupTls(&t.config.Tls); err != nil {
			return err
		}
	}
	if t.config.KeepAlivePeriod != 0 {
		t.keepAliveDuration = time.Duration(t.config.KeepAlivePeriod) * time.Second
	}
	t.stopChan = make(chan bool)
	closeIt = false
	return nil
}

func (t *TcpInput) setupTls(tomlConf *TlsConfig) (err error) {
	if tomlConf.CertFile == "" || tomlConf.KeyFile == "" {
		return errors.New("TLS config requires both cert_file and key_file value.")
	}
	var goConf *tls.Config
	if goConf, err = CreateGoTlsConfig(tomlConf); err == nil {
		t.listener = tls.NewListener(t.listener, goConf)
	}
	return
}

// Listen on the provided TCP connection, extracting messages from the incoming
// data until the connection is closed or Stop is called on the input.
func (t *TcpInput) handleConnection(conn net.Conn) {
	raddr := conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(raddr)
	if err != nil {
		host = raddr
	}

	deliverer := t.ir.NewDeliverer(host)
	sr := t.ir.NewSplitterRunner(host)

	defer func() {
		conn.Close()
		t.wg.Done()
		deliverer.Done()
		sr.Done()
	}()

	if !sr.UseMsgBytes() {
		name := t.ir.Name()
		packDec := func(pack *PipelinePack) {
			pack.Message.SetHostname(raddr)
			pack.Message.SetType(name)
		}
		sr.SetPackDecorator(packDec)
	}

	stopped := false
	for !stopped {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		select {
		case <-t.stopChan:
			stopped = true
		default:
			err = sr.SplitStream(conn, deliverer)
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// keep the connection open, we are just checking to see if
					// we are shutting down: Issue #354
				} else {
					stopped = true
				}
			}
		}
	}
}

func (t *TcpInput) Run(ir InputRunner, h PluginHelper) error {
	t.ir = ir
	var conn net.Conn
	var e error
	for {
		if conn, e = t.listener.Accept(); e != nil {
			if netErr, ok := e.(net.Error); ok && netErr.Temporary() {
				t.ir.LogError(fmt.Errorf("TCP accept failed: %s", e))
				continue
			} else {
				break
			}
		}
		if t.config.KeepAlive {
			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				return errors.New("KeepAlive only supported for TCP Connections.")
			}
			tcpConn.SetKeepAlive(t.config.KeepAlive)
			if t.keepAliveDuration != 0 {
				tcpConn.SetKeepAlivePeriod(t.keepAliveDuration)
			}
		}
		t.wg.Add(1)
		go t.handleConnection(conn)
	}
	t.wg.Wait()
	return nil
}

func (t *TcpInput) Stop() {
	if err := t.listener.Close(); err != nil {
		t.ir.LogError(fmt.Errorf("Error closing listener: %s", err))
	}
	close(t.stopChan)
}

func init() {
	RegisterPlugin("TcpInput", func() interface{} {
		return new(TcpInput)
	})
}

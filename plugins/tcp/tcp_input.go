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
#   Carlos Diaz-Padron (cpadron@mozilla.com,carlos@carlosdp.io)
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
	"crypto/tls"
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/pipeline"
	"net"
	"sync"
	"time"
)

// Input plugin implementation that listens for Heka protocol messages on a
// specified TCP socket. Creates a separate goroutine for each TCP connection.
type TcpInput struct {
	listener net.Listener
	name     string
	wg       sync.WaitGroup
	stopChan chan bool
	ir       InputRunner
	h        PluginHelper
	config   *TcpInputConfig
}

type TcpInputConfig struct {
	// Network type (e.g. "tcp", "tcp4", "tcp6", "unix" or "unixpacket"). Needs to match the input type.
	Net string
	// String representation of the address of the network connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
	// Set of message signer objects, keyed by signer id string.
	Signers map[string]Signer `toml:"signer"`
	// Name of configured decoder to receive the input
	Decoder string
	// Type of parser used to break the stream up into messages
	ParserType string `toml:"parser_type"`
	// Delimiter used to split the stream into messages
	Delimiter string
	// String indicating if the delimiter is at the start or end of the line,
	// only used for regexp delimiters
	DelimiterLocation string `toml:"delimiter_location"`
	// Set to true if the TCP connection should be tunneled through TLS.
	// Requires additional Tls config section.
	UseTls bool `toml:"use_tls"`
	// Subsection for TLS configuration.
	Tls TlsConfig
	// Set to true if TCP Keep Alive should be used.
	KeepAlive bool `toml:"keep_alive"`
	// Integer indicating seconds between keep alives.
	KeepAlivePeriod int `toml:"keep_alive_period"`
}

func (t *TcpInput) ConfigStruct() interface{} {
	config := &TcpInputConfig{Net: "tcp"}
	config.Tls = TlsConfig{PreferServerCiphers: true}
	return config
}

func (t *TcpInput) Init(config interface{}) error {
	var err error
	t.config = config.(*TcpInputConfig)
	address, err := net.ResolveTCPAddr(t.config.Net, t.config.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
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
	if t.config.ParserType == "message.proto" {
		if t.config.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else if t.config.ParserType == "regexp" {
		rp := NewRegexpParser() // temporary parser to test the config
		if len(t.config.Delimiter) > 0 {
			if err = rp.SetDelimiter(t.config.Delimiter); err != nil {
				return err
			}
		}
		if err = rp.SetDelimiterLocation(t.config.DelimiterLocation); err != nil {
			return err
		}
	} else if t.config.ParserType == "token" {
		if len(t.config.Delimiter) > 1 {
			return fmt.Errorf("invalid delimiter: %s", t.config.Delimiter)
		}
	} else {
		return fmt.Errorf("unknown parser type: %s", t.config.ParserType)
	}
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
	defer func() {
		conn.Close()
		t.wg.Done()
	}()

	var (
		dr DecoderRunner
		ok bool
	)
	if t.config.Decoder != "" {
		raddr := conn.RemoteAddr().String()
		host, _, err := net.SplitHostPort(raddr)
		if err != nil {
			host = raddr
		}
		if dr, ok = t.h.DecoderRunner(t.config.Decoder,
			fmt.Sprintf("%s-%s-%s", t.name, host, t.config.Decoder)); !ok {
			t.ir.LogError(fmt.Errorf("Error getting decoder: %s", t.config.Decoder))
			return
		}
	}

	var (
		parser        StreamParser
		parseFunction NetworkParseFunction
	)
	if t.config.ParserType == "message.proto" {
		mp := NewMessageProtoParser()
		parser = mp
		parseFunction = NetworkMessageProtoParser
	} else if t.config.ParserType == "regexp" {
		rp := NewRegexpParser()
		parser = rp
		parseFunction = NetworkPayloadParser
		if len(t.config.Delimiter) > 0 {
			rp.SetDelimiter(t.config.Delimiter)
		}
		rp.SetDelimiterLocation(t.config.DelimiterLocation)
	} else if t.config.ParserType == "token" {
		tp := NewTokenParser()
		parser = tp
		parseFunction = NetworkPayloadParser
		if len(t.config.Delimiter) == 1 {
			tp.SetDelimiter(t.config.Delimiter[0])
		}
	}

	var err error
	stopped := false
	for !stopped {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		select {
		case <-t.stopChan:
			stopped = true
		default:
			err = parseFunction(conn, parser, t.ir, t.config.Signers, dr)
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
	// Stop the decoder, see Issue #713.
	if dr != nil {
		t.h.StopDecoderRunner(dr)
	}
}

func (t *TcpInput) Run(ir InputRunner, h PluginHelper) error {
	t.ir = ir
	t.h = h
	t.stopChan = make(chan bool)
	t.name = ir.Name()

	var conn net.Conn
	var e error
	for {
		if conn, e = t.listener.Accept(); e != nil {
			if e.(net.Error).Temporary() {
				t.ir.LogError(fmt.Errorf("TCP accept failed: %s", e))
				continue
			} else {
				break
			}
		}
		t.wg.Add(1)
		if t.config.KeepAlive {
			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				return errors.New("KeepAlive only supported for TCP Connections.")
			}
			tcpConn.SetKeepAlive(t.config.KeepAlive)
			tcpConn.SetKeepAlivePeriod(time.Duration(t.config.KeepAlivePeriod) * time.Second)
		}

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

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
#
# ***** END LICENSE BLOCK *****/

package tcp

import (
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
	config   *NetworkInputConfig
}

func (t *TcpInput) ConfigStruct() interface{} {
	return &NetworkInputConfig{Net: "tcp"}
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
		if dr, ok = t.h.DecoderRunner(t.config.Decoder); !ok {
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
		rp.SetDelimiter(t.config.Delimiter)
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
			if err = parseFunction(conn, parser, t.ir, t.config, dr); err != nil {
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

func (t *TcpInput) Init(config interface{}) error {
	var err error
	t.config = config.(*NetworkInputConfig)
	t.listener, err = net.Listen(t.config.Net, t.config.Address)
	if err != nil {
		return fmt.Errorf("ListenTCP failed: %s\n", err.Error())
	}
	if t.config.ParserType == "message.proto" {
		if t.config.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else if t.config.ParserType == "regexp" {
		rp := NewRegexpParser() // temporary parser to test the config
		if err = rp.SetDelimiter(t.config.Delimiter); err != nil {
			return err
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
	return nil
}

func (t *TcpInput) Run(ir InputRunner, h PluginHelper) error {
	t.ir = ir
	t.h = h
	t.stopChan = make(chan bool)

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
		go t.handleConnection(conn)
	}
	t.wg.Wait()
	return nil
}

func (t *TcpInput) Stop() {
	t.listener.Close()
	close(t.stopChan)
}

func init() {
	RegisterPlugin("TcpInput", func() interface{} {
		return new(TcpInput)
	})
}

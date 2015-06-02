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
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

	. "github.com/mozilla-services/heka/pipeline"
)

// Output plugin that sends statmetric messages via TCP
type CarbonOutput struct {
	bufSplitSize int
	*CarbonOutputConfig
	*net.UDPAddr
	*net.TCPAddr
	*net.TCPConn
	send func(or OutputRunner, data []byte)
}

// ConfigStruct for CarbonOutput plugin.
type CarbonOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address string
	// Keep the TCP connection open
	TCPKeepAlive bool `toml:"tcp_keep_alive"`
	// If true, use UDP rather than TCP (default) to send the data
	Protocol string `toml:"protocol"`
}

func (t *CarbonOutput) ConfigStruct() interface{} {
	return &CarbonOutputConfig{Address: "localhost:2003"}
}

func (t *CarbonOutput) Init(config interface{}) (err error) {
	t.CarbonOutputConfig = config.(*CarbonOutputConfig)

	switch t.Protocol {
	case "", "tcp":
		t.send = t.sendTCP
		t.TCPAddr, err = net.ResolveTCPAddr("tcp", t.Address)
	case "udp":
		t.send = t.sendUDP
		t.UDPAddr, err = net.ResolveUDPAddr("udp", t.Address)
		t.bufSplitSize = 63488 // 62KiB
	default:
		err = fmt.Errorf(`CarbonOutput: "%s" is not a supported protocol, must be "tcp" or "udp"`, t.Protocol)
	}

	return
}

func (t *CarbonOutput) ProcessPack(pack *PipelinePack, or OutputRunner) {
	var e error

	payload := strings.Trim(pack.Message.GetPayload(), " \t\n")
	// Once we've copied the payload we're done w/ the pack.
	or.UpdateCursor(pack.QueueCursor)
	pack.Recycle(nil)

	lines := strings.Split(payload, "\n")
	clean_statmetrics := make([]string, len(lines))
	index := 0
	for _, line := range lines {
		// `fields` should be "<name> <value> <timestamp>"
		fields := strings.Fields(line)
		if len(fields) != 3 {
			or.LogError(fmt.Errorf("malformed statmetric line: '%s'", line))
			continue
		}

		if _, e = strconv.ParseUint(fields[2], 0, 32); e != nil {
			or.LogError(fmt.Errorf("parsing time: %s", e))
			continue
		}
		if _, e = strconv.ParseFloat(fields[1], 64); e != nil {
			or.LogError(fmt.Errorf("parsing value '%s': %s", fields[1], e))
			continue
		}
		clean_statmetrics[index] = line
		index += 1
	}
	clean_statmetrics = clean_statmetrics[:index]

	// Stuff each parseable statmetric into a bytebuffer
	buffer := &bytes.Buffer{}
	for i := 0; i < len(clean_statmetrics); i++ {
		buffer.WriteString(clean_statmetrics[i] + "\n")
		// UDP packets must be < 64KiB, we cap buffer len at ~62KiB
		if t.bufSplitSize > 0 && buffer.Len() > t.bufSplitSize {
			t.send(or, buffer.Bytes())
			buffer.Reset()
		}
	}

	t.send(or, buffer.Bytes())
}

func (t *CarbonOutput) sendTCP(or OutputRunner, data []byte) {
	write := func() (err error) {
		if t.TCPConn == nil {
			t.TCPConn, err = net.DialTCP("tcp", nil, t.TCPAddr)
			if err != nil {
				or.LogError(fmt.Errorf("Dial failed: %s", err.Error()))
				return
			}
		}
		_, err = t.TCPConn.Write(data)
		if err != nil {
			or.LogError(fmt.Errorf("Write to server failed: %s", err.Error()))
			return
		}
		return
	}

	disconnect := func() {
		if t.TCPConn == nil {
			return
		}
		t.TCPConn.Close()
		t.TCPConn = nil
	}

	if !t.TCPKeepAlive {
		defer disconnect()
	}

	err := write()
	if err == nil {
		return
	}

	if t.TCPKeepAlive {
		// try to reset the connection as it might have gone bad
		or.LogError(fmt.Errorf(`Error "%s", connection reset, retrying`, err.Error()))
		disconnect()
		err = write()
	}
}

func (t *CarbonOutput) sendUDP(or OutputRunner, data []byte) {
	conn, err := net.DialUDP("udp", nil, t.UDPAddr)
	if err != nil {
		or.LogError(fmt.Errorf("Dial failed: %s", err.Error()))
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		or.LogError(fmt.Errorf("Write to server failed: %s", err.Error()))
		return
	}
}

func (t *CarbonOutput) Run(or OutputRunner, h PluginHelper) (err error) {

	var (
		pack *PipelinePack
	)

	for pack = range or.InChan() {
		t.ProcessPack(pack, or)
	}

	return
}

func init() {
	RegisterPlugin("CarbonOutput", func() interface{} {
		return new(CarbonOutput)
	})
}

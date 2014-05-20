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
#
# ***** END LICENSE BLOCK *****/

package udp

import (
	"errors"
	"fmt"
	. "github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Input plugin implementation that listens for Heka protocol messages on a
// specified UDP socket.
type UdpInput struct {
	listener      net.Conn
	name          string
	stopped       bool
	config        *UdpInputConfig
	parser        StreamParser
	parseFunction NetworkParseFunction
}

// ConfigStruct for NetworkInput plugins.
type UdpInputConfig struct {
	// Network type ("udp", "udp4" or "udp6"). Needs to match the input type.
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
}

func (u *UdpInput) ConfigStruct() interface{} {
	return &UdpInputConfig{Net: "udp"}
}

func (u *UdpInput) Init(config interface{}) (err error) {
	u.config = config.(*UdpInputConfig)

	if u.config.Net == "unixgram" {
		if runtime.GOOS == "windows" {
			return errors.New(
				"Can't use Unix datagram sockets on Windows.")
		}
		unixAddr, err := net.ResolveUnixAddr(u.config.Net, u.config.Address)
		if err != nil {
			return fmt.Errorf("Error resolving unixgram address: %s", err)
		}
		u.listener, err = net.ListenUnixgram(u.config.Net, unixAddr)
		if err != nil {
			return fmt.Errorf("Error listening on unixgram: %s", err)
		}
		// Make sure socket file is world writable.
		if err = os.Chmod(u.config.Address, 0666); err != nil {
			return fmt.Errorf("Error changing unixgram socket permissions: ", err)
		}

	} else if len(u.config.Address) > 3 && u.config.Address[:3] == "fd:" {
		// File descriptor
		fdStr := u.config.Address[3:]
		fdInt, err := strconv.ParseUint(fdStr, 0, 0)
		if err != nil {
			return fmt.Errorf("Error parsing file descriptor '%s': %s",
				u.config.Address, err)
		}
		fd := uintptr(fdInt)
		udpFile := os.NewFile(fd, "udpFile")
		u.listener, err = net.FileConn(udpFile)
		if err != nil {
			return fmt.Errorf("Error accessing UDP fd: %s\n", err.Error())
		}
	} else {
		// IP address
		udpAddr, err := net.ResolveUDPAddr(u.config.Net, u.config.Address)
		if err != nil {
			return fmt.Errorf("ResolveUDPAddr failed: %s\n", err.Error())
		}
		u.listener, err = net.ListenUDP(u.config.Net, udpAddr)
		if err != nil {
			return fmt.Errorf("ListenUDP failed: %s\n", err.Error())
		}
	}
	if u.config.ParserType == "message.proto" {
		mp := NewMessageProtoParser()
		u.parser = mp
		u.parseFunction = NetworkMessageProtoParser
		if u.config.Decoder == "" {
			return fmt.Errorf("The message.proto parser must have a decoder")
		}
	} else if u.config.ParserType == "regexp" {
		rp := NewRegexpParser()
		u.parser = rp
		u.parseFunction = NetworkPayloadParser
		if err = rp.SetDelimiter(u.config.Delimiter); err != nil {
			return err
		}
		if err = rp.SetDelimiterLocation(u.config.DelimiterLocation); err != nil {
			return err
		}
	} else if u.config.ParserType == "token" {
		tp := NewTokenParser()
		u.parser = tp
		u.parseFunction = NetworkPayloadParser
		switch len(u.config.Delimiter) {
		case 0: // no value was set, the default provided by the StreamParser will be used
		case 1:
			tp.SetDelimiter(u.config.Delimiter[0])
		default:
			return fmt.Errorf("invalid delimiter: %s", u.config.Delimiter)
		}
	} else {
		return fmt.Errorf("unknown parser type: %s", u.config.ParserType)
	}
	u.parser.SetMinimumBufferSize(1024 * 64)
	return
}

func (u *UdpInput) Run(ir InputRunner, h PluginHelper) error {
	var (
		dr DecoderRunner
		ok bool
	)
	if u.config.Decoder != "" {
		if dr, ok = h.DecoderRunner(u.config.Decoder, fmt.Sprintf("%s-%s",
			ir.Name(), u.config.Decoder)); !ok {

			return fmt.Errorf("Error getting decoder: %s", u.config.Decoder)
		}
	}

	var err error
	for !u.stopped {
		if err = u.parseFunction(u.listener, u.parser, ir, u.config.Signers,
			dr); err != nil {

			if !strings.Contains(err.Error(), "use of closed") {
				ir.LogError(fmt.Errorf("Read error: ", err))
			}
		}
		u.parser.GetRemainingData() // reset the receiving buffer
	}
	return nil
}

func (u *UdpInput) Stop() {
	u.stopped = true
	u.listener.Close()
}

func init() {
	RegisterPlugin("UdpInput", func() interface{} {
		return new(UdpInput)
	})
}

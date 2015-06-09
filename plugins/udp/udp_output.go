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
#
# ***** END LICENSE BLOCK *****/

package udp

import (
	"errors"
	"fmt"
	"net"
	"runtime"

	"github.com/mozilla-services/heka/pipeline"
)

// This is our plugin struct.
type UdpOutput struct {
	*UdpOutputConfig
	conn net.Conn
}

// This is our plugin's config struct
type UdpOutputConfig struct {
	// Network type ("udp", "udp4", "udp6", or "unixgram"). Needs to match the
	// input type.
	Net string
	// String representation of the address of the network connection to which
	// we will be sending out packets (e.g. "192.168.64.48:3336").
	Address string
	// Optional address to use as the local address for the connection.
	LocalAddress string `toml:"local_address"`

	// Maximum size of message, plugin drops the data if it exceeds this limit.
	MaxMessageSize int `toml:"max_message_size"`
}

// Provides pipeline.HasConfigStruct interface.
func (o *UdpOutput) ConfigStruct() interface{} {
	return &UdpOutputConfig{
		Net: "udp",

		// Defines maximum size of udp data for IPv4.
		MaxMessageSize: 65507,
	}
}

// Initialize UDP connection
func (o *UdpOutput) Init(config interface{}) (err error) {
	o.UdpOutputConfig = config.(*UdpOutputConfig) // assert we have the right config type

	if o.UdpOutputConfig.MaxMessageSize < 512 {
		return fmt.Errorf("Maximum message size can't be smaller than 512 bytes.")
	}

	if o.Net == "unixgram" {
		if runtime.GOOS == "windows" {
			return errors.New("Can't use Unix datagram sockets on Windows.")
		}
		var unixAddr, lAddr *net.UnixAddr
		unixAddr, err = net.ResolveUnixAddr(o.Net, o.Address)
		if err != nil {
			return fmt.Errorf("Error resolving unixgram address '%s': %s", o.Address,
				err.Error())
		}
		if o.LocalAddress != "" {
			lAddr, err = net.ResolveUnixAddr(o.Net, o.LocalAddress)
			if err != nil {
				return fmt.Errorf("Error resolving local unixgram address '%s': %s",
					o.LocalAddress, err.Error())
			}
		}
		if o.conn, err = net.DialUnix(o.Net, lAddr, unixAddr); err != nil {
			return fmt.Errorf("Can't connect to '%s': %s", o.Address,
				err.Error())
		}
	} else {
		var udpAddr, lAddr *net.UDPAddr
		if udpAddr, err = net.ResolveUDPAddr(o.Net, o.Address); err != nil {
			return fmt.Errorf("Error resolving UDP address '%s': %s", o.Address,
				err.Error())
		}
		if o.LocalAddress != "" {
			lAddr, err = net.ResolveUDPAddr(o.Net, o.LocalAddress)
			if err != nil {
				return fmt.Errorf("Error resolving local UDP address '%s': %s",
					o.Address, err.Error())
			}
		}
		if o.conn, err = net.DialUDP(o.Net, lAddr, udpAddr); err != nil {
			return fmt.Errorf("Can't connect to '%s': %s", o.Address,
				err.Error())
		}
	}
	return
}

func (o *UdpOutput) Run(or pipeline.OutputRunner, h pipeline.PluginHelper) (err error) {

	if or.Encoder() == nil {
		return errors.New("Encoder required.")
	}

	var (
		outBytes []byte
		e        error
	)

	for pack := range or.InChan() {
		if outBytes, e = or.Encode(pack); e != nil {
			or.UpdateCursor(pack.QueueCursor)
			e = fmt.Errorf("Error encoding message: %s", e.Error())
			pack.Recycle(e)
			continue
		} else if outBytes != nil {
			msgSize := len(outBytes)
			if msgSize > o.UdpOutputConfig.MaxMessageSize {
				or.UpdateCursor(pack.QueueCursor)
				e = fmt.Errorf("Message has exceeded allowed UDP data size: %d > %d",
					msgSize, o.UdpOutputConfig.MaxMessageSize)
				pack.Recycle(e)
				continue
			} else {
				o.conn.Write(outBytes)
			}
		}
		or.UpdateCursor(pack.QueueCursor)
		pack.Recycle(nil)
	}
	return
}

func init() {
	pipeline.RegisterPlugin("UdpOutput", func() interface{} {
		return new(UdpOutput)
	})
}

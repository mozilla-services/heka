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
	"os"
	"runtime"
	"strconv"
	"strings"

	. "github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
)

// Input plugin implementation that listens for Heka protocol messages on a
// specified UDP socket.
type UdpInput struct {
	listener net.Conn
	name     string
	stopChan chan struct{}
	config   *UdpInputConfig
}

// ConfigStruct for NetworkInput plugins.
type UdpInputConfig struct {
	// Network type ("udp", "udp4", "udp6", or "unixgram"). Needs to match the
	// input type.
	Net string
	// String representation of the address of the network connection on which
	// the listener should be listening (e.g. "127.0.0.1:5565").
	Address string
}

func (u *UdpInput) ConfigStruct() interface{} {
	return &UdpInputConfig{
		Net: "udp",
	}
}

func (u *UdpInput) Init(config interface{}) (err error) {
	u.config = config.(*UdpInputConfig)

	if u.config.Net == "unixgram" {
		if runtime.GOOS == "windows" {
			return errors.New(
				"Can't use Unix datagram sockets on Windows.")
		}
		if runtime.GOOS != "linux" && strings.HasPrefix(u.config.Address, "@") {
			return errors.New(
				"Abstract sockets are linux-specific.")
		}
		unixAddr, err := net.ResolveUnixAddr(u.config.Net, u.config.Address)
		if err != nil {
			return fmt.Errorf("Error resolving unixgram address: %s", err)
		}
		u.listener, err = net.ListenUnixgram(u.config.Net, unixAddr)
		if err != nil {
			return fmt.Errorf("Error listening on unixgram: %s", err)
		}
		// Ensure socket file is world writable, unless socket is abstract.
		if !strings.HasPrefix(u.config.Address, "@") {
			if err = os.Chmod(u.config.Address, 0666); err != nil {
				return fmt.Errorf(
					"Error changing unixgram socket permissions: %s", err)
			}
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
	u.stopChan = make(chan struct{})
	return
}

func (u *UdpInput) Run(ir InputRunner, h PluginHelper) error {
	sr := ir.NewSplitterRunner("")
	defer sr.Done()
	ok := true
	var err error

	if !sr.UseMsgBytes() {
		name := ir.Name()
		packDec := func(pack *PipelinePack) {
			pack.Message.SetType(name)
		}
		sr.SetPackDecorator(packDec)
	}

	for ok {
		select {
		case _, ok = <-u.stopChan:
			break
		default:
			err = sr.SplitStream(u.listener, nil)
			// "use of closed" -> we're stopping.
			if err != nil && !strings.Contains(err.Error(), "use of closed") {
				ir.LogError(fmt.Errorf("Read error: %s", err))
			}
			sr.GetRemainingData() // reset the receiving buffer
		}
	}
	if u.config.Net == "unixgram" {
		if !strings.HasPrefix(u.config.Address, "@") {
			err = os.Remove(u.config.Address)
			if err != nil {
				ir.LogError(errors.New("Error cleaning up unix datagram socket"))
			}
		}
	}
	return nil
}

func (u *UdpInput) Stop() {
	close(u.stopChan)
	u.listener.Close()
}

func init() {
	RegisterPlugin("UdpInput", func() interface{} {
		return new(UdpInput)
	})
}

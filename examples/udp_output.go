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

package examples

import (
	"fmt"
	"github.com/mozilla-services/heka/pipeline"
	"net"
)

// This is our plugin struct.
type UdpOutput struct {
	conn net.Conn
}

// This is our plugin's config struct
type UdpOutputConfig struct {
	Address string
}

// Provides pipeline.HasConfigStruct interface.
func (o *UdpOutput) ConfigStruct() interface{} {
	return &UdpOutputConfig{"my.example.com:44444"}
}

// Initialize UDP connection
func (o *UdpOutput) Init(config interface{}) (err error) {
	conf := config.(*UdpOutputConfig) // assert we have the right config type
	var udpAddr *net.UDPAddr
	if udpAddr, err = net.ResolveUDPAddr("udp", conf.Address); err != nil {
		return fmt.Errorf("can't resolve %s: %s", conf.Address,
			err.Error())
	}
	if o.conn, err = net.DialUDP("udp", nil, udpAddr); err != nil {
		return fmt.Errorf("error dialing %s: %s", conf.Address,
			err.Error())
	}
	return
}

func (o *UdpOutput) Run(runner pipeline.FilterRunner, helper pipeline.PluginHelper) (
	err error) {

	var outgoing string
	for pack := range runner.InChan() {
		outgoing = fmt.Sprintf("%s\n", pack.Message.GetPayload())
		o.conn.Write([]byte(outgoing))
		pack.Recycle()
	}
	return
}

func init() {
	pipeline.RegisterPlugin("UdpOutput", func() interface{} {
		return new(UdpOutput)
	})
}

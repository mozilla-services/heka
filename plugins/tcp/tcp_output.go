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
	. "github.com/mozilla-services/heka/pipeline"
	"net"
)

// Output plugin that sends messages via TCP using the Heka protocol.
type TcpOutput struct {
	address    string
	connection net.Conn
}

// ConfigStruct for TcpOutput plugin.
type TcpOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address string
	UseTls  bool `toml:"use_tls"`
	Tls     TlsConfig
}

func (t *TcpOutput) ConfigStruct() interface{} {
	return &TcpOutputConfig{Address: "localhost:9125"}
}

func (t *TcpOutput) Init(config interface{}) (err error) {
	conf := config.(*TcpOutputConfig)
	t.address = conf.Address
	if conf.UseTls {
		var goTlsConf *tls.Config
		if goTlsConf, err = CreateGoTlsConfig(&conf.Tls); err != nil {
			return fmt.Errorf("TLS init error: %s", err)
		}
		t.connection, err = tls.Dial("tcp", t.address, goTlsConf)
	} else {
		t.connection, err = net.Dial("tcp", t.address)
	}
	return
}

func (t *TcpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var e error
	var n int
	outBytes := make([]byte, 0, 2000)

	for pack := range or.InChan() {
		outBytes = outBytes[:0]

		if e = ProtobufEncodeMessage(pack, &outBytes); e != nil {
			or.LogError(e)
			pack.Recycle()
			continue
		}

		if n, e = t.connection.Write(outBytes); e != nil {
			or.LogError(fmt.Errorf("writing to %s: %s", t.address, e))
		} else if n != len(outBytes) {
			or.LogError(fmt.Errorf("truncated output to: %s", t.address))
		}

		pack.Recycle()
	}

	t.connection.Close()

	return
}

func init() {
	RegisterPlugin("TcpOutput", func() interface{} {
		return new(TcpOutput)
	})
}

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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Output plugin that sends statmetric messages via TCP
type CarbonOutput struct {
	address string
}

// ConfigStruct for CarbonOutput plugin.
type CarbonOutputConfig struct {
	// String representation of the TCP address to which this output should be
	// sending data.
	Address string
}

func (t *CarbonOutput) ConfigStruct() interface{} {
	return &CarbonOutputConfig{Address: "localhost:9125"}
}

func (t *CarbonOutput) Init(config interface{}) (err error) {
	conf := config.(*CarbonOutputConfig)
	t.address = conf.Address

	_, err = net.ResolveTCPAddr("tcp", t.address)
	if err != nil {
		return
	}

	return
}

func (t *CarbonOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	var e error

	var (
		fields []string
		pack   *PipelinePack
	)

	for plc := range or.InChan() {
		pack = plc.Pack
		payload := pack.Message.GetPayload()
		pack.Recycle() // Once we've copied the payload we're done w/ the pack.
		lines := strings.Split(strings.Trim(payload, " \n"), "\n")

		for _, line := range lines {
			// `fields` should be "<name> <value> <timestamp>"
			fields = strings.Fields(line)
			if len(fields) != 3 || !strings.HasPrefix(fields[0], "stats") {
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

			conn, err := net.Dial("tcp", t.address)
			if err != nil {
				or.LogError(fmt.Errorf("Dial failed: %s",
					err.Error()))
				continue
			}
			defer conn.Close()

			_, err = conn.Write([]byte(line))
			if err != nil {
				or.LogError(fmt.Errorf("Write to server failed: %s",
					err.Error()))
				continue
			}
		}
	}

	return
}

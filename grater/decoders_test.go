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
package hekagrater

import (
	"heka/client"
	"os"
	"log"
	"time"
	"github.com/orfjackal/gospec/src/gospec"
	gs "github.com/orfjackal/gospec/src/gospec"
)

func DecodersSpec(c gospec.Context) {

	timestamp := time.Now()
	hostname, _ := os.Hostname()
	fields := make(map[string]interface{})
	fields["foo"] = "bar"
	origMsg := hekaclient.Message{
		Type: "TEST", Timestamp: timestamp,
		Logger: "DecodersSpec", Severity: 6,
		Payload: "Test Payload", Env_version: "0.8",
		Pid: os.Getpid(), Hostname: hostname,
		Fields: fields,
	}

	pipelinePack := &PipelinePack{}

	c.Specify("A JsonDecoder object", func() {
		jsonEncoder := &hekaclient.JsonEncoder{}
		var err error
		msgBytes, err := jsonEncoder.EncodeMessage(&origMsg)
		pipelinePack.MsgBytes = msgBytes
		c.Assume(err, gs.IsNil)

		jsonDecoder := &JsonDecoder{}

		c.Specify("can decode valid JSON", func() {
			log.Println(pipelinePack)
			decodedMsg := jsonDecoder.Decode(pipelinePack)
			c.Expect(decodedMsg, gs.Equals, &origMsg)
		})
	})
}
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
package pipeline

import (
	"github.com/orfjackal/gospec/src/gospec"
	. "heka/message"
	"heka/testsupport"
	"os"
	"testing"
	"time"
)

var config = GraterConfig{DefaultDecoder: "TEST", DefaultFilterChain: "TEST"}

func TestAllSpecs(t *testing.T) {
	testsupport.SetTestingT(t)
	r := gospec.NewRunner()
	r.AddSpec(DecodersSpec)
	r.AddSpec(InputsSpec)
	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	timestamp := time.Now()
	hostname, _ := os.Hostname()
	fields := make(map[string]interface{})
	fields["foo"] = "bar"
	msg := Message{
		Type: "TEST", Timestamp: timestamp,
		Logger: "GoSpec", Severity: 6,
		Payload: "Test Payload", Env_version: "0.8",
		Pid: os.Getpid(), Hostname: hostname,
		Fields: fields,
	}
	return &msg
}

func getTestPipelinePack() *PipelinePack {
	return NewPipelinePack(&config)
}

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
	hekatime "heka/time"
	"os"
	"testing"
	"time"
)

func mockDecoderCreator() map[string]Decoder {
	return make(map[string]Decoder)
}

func mockFilterCreator() map[string]Filter {
	return make(map[string]Filter)
}

func mockOutputCreator() map[string]Output {
	return make(map[string]Output)
}

var config = PipelineConfig{DefaultDecoder: "TEST", DefaultFilterChain: "TEST",
	DecoderCreator: mockDecoderCreator, FilterCreator: mockFilterCreator,
	OutputCreator: mockOutputCreator}

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.AddSpec(DecodersSpec)
	r.AddSpec(InputsSpec)
	r.AddSpec(InputRunnerSpec)
	r.AddSpec(OutputsSpec)
	r.AddSpec(StatsdOutputsSpec)
	r.AddSpec(LoadFromConfigSpec)
	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	timestamp := hekatime.UTCTimestamp{time.Now().UTC()}
	hostname, _ := os.Hostname()
	fields := make(map[string]interface{})
	fields["foo"] = "bar"
	msg := Message{
		Type:      "TEST",
		Timestamp: timestamp,
		Logger:    "GoSpec", Severity: 6,
		Payload: "Test Payload", Env_version: "0.8",
		Pid: os.Getpid(), Hostname: hostname,
		Fields: fields,
	}
	return &msg
}

func getTestPipelinePack() *PipelinePack {
	return NewPipelinePack(&config)
}

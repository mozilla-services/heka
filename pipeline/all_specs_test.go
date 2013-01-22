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
	"code.google.com/p/go-uuid/uuid"
	. "github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/gospec/src/gospec"
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

var config = PipelineConfig{DefaultDecoder: "TEST", DefaultFilterChain: "TEST"}

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.Parallel = false
	r.AddSpec(DecodersSpec)
	r.AddSpec(InputsSpec)
	r.AddSpec(InputRunnerSpec)
	r.AddSpec(OutputsSpec)
	r.AddSpec(LoadFromConfigSpec)
	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	hostname, _ := os.Hostname()
	field, _ := NewField("foo", "bar", Field_RAW)
	msg := NewMessage()
	*msg.Type = "TEST"
	*msg.Timestamp = time.Now().UnixNano()
	u := uuid.NewRandom()
	copy(msg.Uuid, u)
	*msg.Logger = "GoSpec"
	*msg.Severity = int32(6)
	*msg.Payload = "Test Payload"
	*msg.EnvVersion = "0.8"
	*msg.Pid = int32(os.Getpid())
	*msg.Hostname = hostname
	msg.AddField(field)

	return msg
}

func getTestPipelinePack() *PipelinePack {
	return NewPipelinePack(&config)
}

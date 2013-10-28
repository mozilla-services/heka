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

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.Parallel = false

	r.AddSpec(DecodersSpec)
	r.AddSpec(FiltersSpec)
	r.AddSpec(OutputsSpec)
	r.AddSpec(LoadFromConfigSpec)
	r.AddSpec(WhisperRunnerSpec)
	r.AddSpec(WhisperOutputSpec)
	r.AddSpec(ReportSpec)
	r.AddSpec(AMQPPluginSpec)
	r.AddSpec(StatsdInputSpec)
	r.AddSpec(InputsSpec)
	r.AddSpec(FileMonitorSpec)
	r.AddSpec(LogfileInputSpec)
	r.AddSpec(StatAccumInputSpec)
	r.AddSpec(CarbonOutputSpec)
	r.AddSpec(DashboardOutputSpec)
	r.AddSpec(JsonPathSpec)
	r.AddSpec(HttpInputSpec)
	r.AddSpec(ElasticSearchOutputSpec)
	r.AddSpec(StreamParserSpec)
	r.AddSpec(ProcessChainSpec)

	gospec.MainGoTest(r, t)
}

func getTestMessage() *Message {
	hostname := "my.host.name"
	field, _ := NewField("foo", "bar", "")
	msg := &Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)
	return msg
}

func BenchmarkPipelinePackCreation(b *testing.B) {
	var config = PipelineConfig{}
	for i := 0; i < b.N; i++ {
		NewPipelinePack(config.inputRecycleChan)
	}
}

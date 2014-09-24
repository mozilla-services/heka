/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package template

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gs.NewRunner()
	r.Parallel = false
	r.AddSpec(TextTemplateEncoderSpec)

	gs.MainGoTest(r, t)
}

func TextTemplateEncoderSpec(c gs.Context) {
	c.Specify("A TextTemplateEncoder", func() {
		encoder := new(TextTemplateEncoder)
		globals := pipeline.DefaultGlobals()
		encoder.pConfig = pipeline.NewPipelineConfig(globals)
		config := encoder.ConfigStruct().(*TextTemplateEncoderConfig)

		supply := make(chan *pipeline.PipelinePack, 1)
		pack := pipeline.NewPipelinePack(supply)
		uuidStr := "123e4567-e89b-12d3-a456-426655440000"
		pack.Message.SetUuid(uuid.Parse(uuidStr))
		tsString := "2014-09-24T11:40:40-07:00"
		ts, err := time.Parse(config.TimestampFormat, tsString)
		c.Assume(err, gs.IsNil)
		pack.Message.SetTimestamp(ts.UnixNano())
		pack.Message.SetType("MyType")
		pack.Message.SetLogger("MyLogger")
		pack.Message.SetSeverity(4)
		pack.Message.SetPayload("This is a payload.")
		pack.Message.SetEnvVersion("0.8")
		pack.Message.SetPid(123)
		pack.Message.SetHostname("test.example.com")
		field, err := message.NewField("foo", "bar", "")
		c.Assume(err, gs.IsNil)
		pack.Message.AddField(field)
		field, err = message.NewField("snarf", 12.34, "")
		c.Assume(err, gs.IsNil)
		pack.Message.AddField(field)

		c.Specify("applies a message to a template", func() {
			config.TemplateFiles = []string{"./testsupport/template.txt"}
			err := encoder.Init(config)
			c.Assume(err, gs.IsNil)

			output, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			expected := `=Test message template=
UUID: 123e4567-e89b-12d3-a456-426655440000
Timestamp: 2014-09-24T11:40:40-07:00
Type: MyType
Logger: MyLogger
Severity: 4
Payload: This is a payload.
EnvVersion: 0.8
Pid: 123
Hostname: test.example.com
foo: bar
snarf: 12.34
`
			c.Expect(string(output), gs.Equals, expected)
		})
	})
}

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

package plugins

import (
	"time"

	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"github.com/pborman/uuid"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func RstEncoderSpec(c gs.Context) {

	c.Specify("A RstEncoder", func() {
		encoder := new(RstEncoder)
		supply := make(chan *pipeline.PipelinePack, 1)

		pack := pipeline.NewPipelinePack(supply)
		payload := "This is the payload!"
		pack.Message.SetPayload(payload)
		loc, err := time.LoadLocation("America/Chicago")
		c.Assume(err, gs.IsNil)
		timestamp := time.Date(1971, 10, 7, 18, 47, 0, 0, loc)
		pack.Message.SetTimestamp(timestamp.UnixNano())
		pack.Message.SetType("test.type")
		pack.Message.SetHostname("somehost.example.com")
		pack.Message.SetPid(12345)
		pack.Message.SetUuid(uuid.Parse("72de6a05-1b99-4a88-84c2-90797624c68f"))

		pack.Message.SetLogger("loggyloglog")
		pack.Message.SetEnvVersion("0.8")

		field, err := message.NewField("intfield", 23, "count")
		c.Assume(err, gs.IsNil)
		field.AddValue(24)
		field.AddValue(25)
		pack.Message.AddField(field)

		field, err = message.NewField("stringfield", "hold", "foreigner")
		c.Assume(err, gs.IsNil)
		field.AddValue("your")
		field.AddValue("head")
		field.AddValue("up")
		pack.Message.AddField(field)

		field, err = message.NewField("bool", true, "")
		c.Assume(err, gs.IsNil)
		field.AddValue(false)
		pack.Message.AddField(field)

		field, err = message.NewField("bool", false, "false")
		c.Assume(err, gs.IsNil)
		pack.Message.AddField(field)

		field, err = message.NewField("float", 345.6789, "kelvin")
		c.Assume(err, gs.IsNil)
		field.AddValue(139847987987987.878732819)
		pack.Message.AddField(field)

		field, err = message.NewField("bytes", []byte("encode me"), "binary")
		c.Assume(err, gs.IsNil)
		field.AddValue([]byte("and me"))
		field.AddValue([]byte("and me too"))
		pack.Message.AddField(field)

		expected := `
:Timestamp: 1971-10-07 23:47:00 +0000 UTC
:Type: test.type
:Hostname: somehost.example.com
:Pid: 12345
:Uuid: 72de6a05-1b99-4a88-84c2-90797624c68f
:Logger: loggyloglog
:Payload: This is the payload!
:EnvVersion: 0.8
:Severity: 7
:Fields:
    | name:"intfield" type:integer value:[23,24,25] representation:"count"
    | name:"stringfield" type:string value:["hold","your","head","up"] representation:"foreigner"
    | name:"bool" type:bool value:[true,false]
    | name:"bool" type:bool value:false representation:"false"
    | name:"float" type:double value:[345.6789,1.3984798798798788e+14] representation:"kelvin"
    | name:"bytes" type:bytes value:[ZW5jb2RlIG1l,YW5kIG1l,YW5kIG1lIHRvbw==] representation:"binary"

`
		c.Specify("serializes a message correctly", func() {
			err = encoder.Init(nil)
			c.Assume(err, gs.IsNil)
			output, err := encoder.Encode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(string(output), gs.Equals, expected)
		})
	})
}

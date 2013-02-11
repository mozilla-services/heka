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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func FilterSpecificationSpec(c gospec.Context) {
	pack := getTestPipelinePack()
	pack.Message = getTestMessage()
	uuidStr := pack.Message.GetUuidString()
	data := []byte("data")
	field1, _ := message.NewField("bytes", data, message.Field_RAW)
	field2, _ := message.NewField("int", int64(999), message.Field_RAW)
	field2.AddValue(int64(1024))
	field3, _ := message.NewField("double", float64(99.9), message.Field_RAW)
	field4, _ := message.NewField("bool", true, message.Field_RAW)
	field5, _ := message.NewField("foo", "alternate", message.Field_RAW)
	pack.Message.AddField(field1)
	pack.Message.AddField(field2)
	pack.Message.AddField(field3)
	pack.Message.AddField(field4)
	pack.Message.AddField(field5)

	c.Specify("A FilterSpecification", func() {
		malformed := []string{"",
			"bogus",
			"Type = \"test\"",                                                 // invalid operator
			"Pid == \"test=\"",                                                // Pid is not a string
			"Type == \"test\" && (Severity==7 || Payload == \"Test Payload\"", // missing paren
			"Invalid == \"bogus\"",                                            // unknown variable name
			"Fields[]",                                                        // empty name key
			"Fields[test][]",                                                  // empty field index
			"Fields[test][a]",                                                 // non numeric field index
			"Fields[test][0][]",                                               // empty array index
			"Fields[test][0][a]",                                              // non numeric array index
			"Fields[test][0][0][]",                                            // extra index dimension
			"Fields[test][xxxx",                                               // unmatched bracket
		}

		negative := []string{"FALSE",
			"Type == \"test\"&&(Severity==7||Payload==\"Test Payload\")",
			"EnvVersion == \"0.9\"",
			"EnvVersion != \"0.8\"",
			"EnvVersion > \"0.9\"",
			"EnvVersion >= \"0.9\"",
			"EnvVersion < \"0.8\"",
			"EnvVersion <= \"0.7\"",
			"Severity == 5",
			"Severity != 6",
			"Severity < 6",
			"Severity <= 5",
			"Severity > 6",
			"Severity >= 7",
			"Fields[foo] == \"ba\"",
			"Fields[foo][1] == \"bar\"",
			"Fields[foo][0][1] == \"bar\"",
			"Fields[bool] == FALSE",
		}

		positive := []string{"TRUE",
			"(Severity == 7 || Payload == \"Test Payload\") && Type == \"TEST\"",
			"EnvVersion == \"0.8\"",
			"EnvVersion != \"0.9\"",
			"EnvVersion > \"0.7\"",
			"EnvVersion >= \"0.8\"",
			"EnvVersion < \"0.9\"",
			"EnvVersion <= \"0.8\"",
			"Hostname != \"\"",
			"Logger == \"GoSpec\"",
			"Pid != 0",
			"Severity != 5",
			"Severity < 7",
			"Severity <= 6",
			"Severity == 6",
			"Severity > 5",
			"Severity >= 6",
			"Timestamp > 0",
			"Type != \"test\"",
			"Type == \"TEST\" && Severity == 6",
			"Type == \"test\" && Severity == 7 || Payload == \"Test Payload\"",
			"Type == \"TEST\"",
			"Type == \"foo\" || Type == \"bar\" || Type == \"TEST\"",
			fmt.Sprintf("Uuid == \"%s\"", uuidStr),
			"Fields[foo] == \"bar\"",
			"Fields[foo][0] == \"bar\"",
			"Fields[foo][0][0] == \"bar\"",
			"Fields[foo][1] == \"alternate\"",
			"Fields[foo][1][0] == \"alternate\"",
			"Fields[foo] == \"bar\"",
			"Fields[bytes] == \"data\"",
			"Fields[int] == 999",
			"Fields[int][0][1] == 1024",
			"Fields[double] == 99.9",
			"Fields[bool] == TRUE",
		}

		c.Specify("malformed filter tests", func() {
			for _, v := range malformed {
				_, err := CreateFilterSpecification(v)
				c.Expect(err, gs.Not(gs.IsNil))
			}
		})

		c.Specify("negative filter tests", func() {
			for _, v := range negative {
				fs, err := CreateFilterSpecification(v)
				c.Expect(err, gs.IsNil)
				fs.FilterMsg(pack)
				c.Expect(pack.Blocked, gs.IsTrue)
			}
		})

		c.Specify("positive filter tests", func() {
			for _, v := range positive {
				fs, err := CreateFilterSpecification(v)
				c.Expect(err, gs.IsNil)
				fs.FilterMsg(pack)
				c.Expect(pack.Blocked, gs.IsFalse)
			}
		})

	})
}

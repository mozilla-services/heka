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

package message

import (
	"fmt"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func MatcherSpecificationSpec(c gospec.Context) {
	msg := getTestMessage()
	uuidStr := msg.GetUuidString()
	data := []byte("data")
	field1, _ := NewField("bytes", data, Field_RAW)
	field2, _ := NewField("int", int64(999), Field_RAW)
	field2.AddValue(int64(1024))
	field3, _ := NewField("double", float64(99.9), Field_RAW)
	field4, _ := NewField("bool", true, Field_RAW)
	field5, _ := NewField("foo", "alternate", Field_RAW)
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)
	msg.AddField(field4)
	msg.AddField(field5)

	c.Specify("A MatcherSpecification", func() {
		malformed := []string{"",
			"bogus",
			"Type = 'test'",                                               // invalid operator
			"Pid == 'test='",                                              // Pid is not a string
			"Type == 'test' && (Severity==7 || Payload == 'Test Payload'", // missing paren
			"Invalid == 'bogus'",                                          // unknown variable name
			"Fields[]",                                                    // empty name key
			"Fields[test][]",                                              // empty field index
			"Fields[test][a]",                                             // non numeric field index
			"Fields[test][0][]",                                           // empty array index
			"Fields[test][0][a]",                                          // non numeric array index
			"Fields[test][0][0][]",                                        // extra index dimension
			"Fields[test][xxxx",                                           // unmatched bracket
			"Pid =~ /6/",                                                  // regex not allowed on numeric
			"Pid !~ /6/",                                                  // regex not allowed on numeric
			"Type =~ /test",                                               // unmatched slash
			"Type == /test/",                                              // incorrect operator
			"Type =~ 'test'",                                              // string instead of regexp
			"Type =~ /\\ytest/",                                           // invalid escape character
			"Type != 'test\"",                                             // mis matched quote types
			"Pid =~ 6",                                                    // number instead of regexp
		}

		negative := []string{"FALSE",
			"Type == 'test'&&(Severity==7||Payload=='Test Payload')",
			"EnvVersion == '0.9'",
			"EnvVersion != '0.8'",
			"EnvVersion > '0.9'",
			"EnvVersion >= '0.9'",
			"EnvVersion < '0.8'",
			"EnvVersion <= '0.7'",
			"Severity == 5",
			"Severity != 6",
			"Severity < 6",
			"Severity <= 5",
			"Severity > 6",
			"Severity >= 7",
			"Fields[foo] == 'ba'",
			"Fields[foo][1] == 'bar'",
			"Fields[foo][0][1] == 'bar'",
			"Fields[bool] == FALSE",
			"Type =~ /Test/",
			"Type !~ /TEST/",
			"Payload =~ /^Payload/",
			"Type == \"te'st\"",
			"Type == 'te\"st'",
			"Fields[int] =~ /999/",
		}

		positive := []string{"TRUE",
			"(Severity == 7 || Payload == 'Test Payload') && Type == 'TEST'",
			"EnvVersion == \"0.8\"",
			"EnvVersion == '0.8'",
			"EnvVersion != '0.9'",
			"EnvVersion > '0.7'",
			"EnvVersion >= '0.8'",
			"EnvVersion < '0.9'",
			"EnvVersion <= '0.8'",
			"Hostname != ''",
			"Logger == 'GoSpec'",
			"Pid != 0",
			"Severity != 5",
			"Severity < 7",
			"Severity <= 6",
			"Severity == 6",
			"Severity > 5",
			"Severity >= 6",
			"Timestamp > 0",
			"Type != 'test'",
			"Type == 'TEST' && Severity == 6",
			"Type == 'test' && Severity == 7 || Payload == 'Test Payload'",
			"Type == 'TEST'",
			"Type == 'foo' || Type == 'bar' || Type == 'TEST'",
			fmt.Sprintf("Uuid == '%s'", uuidStr),
			"Fields[foo] == 'bar'",
			"Fields[foo][0] == 'bar'",
			"Fields[foo][0][0] == 'bar'",
			"Fields[foo][1] == 'alternate'",
			"Fields[foo][1][0] == 'alternate'",
			"Fields[foo] == 'bar'",
			"Fields[bytes] == 'data'",
			"Fields[int] == 999",
			"Fields[int][0][1] == 1024",
			"Fields[double] == 99.9",
			"Fields[bool] == TRUE",
			"Type =~ /TEST/",
			"Type !~ /bogus/",
			"Type =~ /TEST/ && Payload =~ /Payload/",
			"Fields[foo][1] =~ /alt/",
		}

		c.Specify("malformed filter tests", func() {
			for _, v := range malformed {
				_, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.Not(gs.IsNil))
			}
		})

		c.Specify("negative filter tests", func() {
			for _, v := range negative {
				ms, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.IsNil)
				c.Expect(ms.IsMatch(msg), gs.IsFalse)
			}
		})

		c.Specify("positive filter tests", func() {
			for _, v := range positive {
				ms, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.IsNil)
				c.Expect(ms.IsMatch(msg), gs.IsTrue)
			}
		})

	})
}

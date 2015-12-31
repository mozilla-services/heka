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
	"testing"
)

func compareCaptures(c gospec.Context, m1, m2 map[string]string) {
	for k, v := range m1 {
		v1, _ := m2[k]
		c.Expect(v, gs.Equals, v1)
	}
}

func MatcherSpecificationSpec(c gospec.Context) {
	msg := getTestMessage()
	uuidStr := msg.GetUuidString()
	data := []byte("data")
	date := "Mon Jan 02 15:04:05 -0700 2006"
	field1, _ := NewField("bytes", data, "")
	field2, _ := NewField("int", int64(999), "")
	field2.AddValue(int64(1024))
	field3, _ := NewField("double", float64(99.9), "")
	field4, _ := NewField("bool", true, "")
	field5, _ := NewField("foo", "alternate", "")
	field6, _ := NewField("Payload", "name=test;type=web;", "")
	field7, _ := NewField("Timestamp", date, "date-time")
	field8, _ := NewField("zero", int64(0), "")
	field9, _ := NewField("string", "43", "")
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)
	msg.AddField(field4)
	msg.AddField(field5)
	msg.AddField(field6)
	msg.AddField(field7)
	msg.AddField(field8)
	msg.AddField(field9)

	c.Specify("A MatcherSpecification", func() {
		malformed := []string{
			"",
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
			"NIL",                                                         // invalid use of constant
			"Type == NIL",                                                 // existence check only works on fields
			"Fields[test] > NIL",                                          // existence check only works with equals and not equals
		}

		negative := []string{
			"FALSE",
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
			"Fields[zero] == \"0\"",
			"Fields[string] == 43",
			"Fields[int] == NIL",
			"Fields[int][0][1] == NIL",
			"Fields[missing] != NIL",
			"Type =~ /^te/",
			"Type =~ /st$/",
			"Type !~ /^TE/",
			"Type !~ /ST$/",
			"Logger =~ /./ && Type =~ /^anything/",
		}

		positive := []string{
			"TRUE",
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
			"Fields[Payload] =~ /name=\\w+/",
			"Type =~ /(ST)/",
			"Fields[int] != NIL",
			"Fields[int][0][1] != NIL",
			"Fields[int][0][2] == NIL",
			"Fields[missing] == NIL",
			"Type =~ /^TE/",
			"Type =~ /ST$/",
			"Type !~ /^te/",
			"Type !~ /st$/",
		}

		c.Specify("malformed matcher tests", func() {
			for _, v := range malformed {
				_, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.Not(gs.IsNil))
			}
		})

		c.Specify("negative matcher tests", func() {
			for _, v := range negative {
				ms, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.IsNil)
				match := ms.Match(msg)
				c.Expect(match, gs.IsFalse)
			}
		})

		c.Specify("positive matcher tests", func() {
			for _, v := range positive {
				ms, err := CreateMatcherSpecification(v)
				c.Expect(err, gs.IsNil)
				match := ms.Match(msg)
				c.Expect(match, gs.IsTrue)
			}
		})
	})
}

func BenchmarkMatcherCreate(b *testing.B) {
	s := "Type == 'Test' && Severity == 6"
	for i := 0; i < b.N; i++ {
		CreateMatcherSpecification(s)
	}
}

func BenchmarkMatcherMatch(b *testing.B) {
	b.StopTimer()
	s := "Type == 'TEST' && Severity == 6"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherSimpleRegex(b *testing.B) {
	b.StopTimer()
	s := "Type =~ /[Tt]EST/ && Severity == 6"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherSimpleRegexCapture(b *testing.B) {
	b.StopTimer()
	s := "Type =~ /^(TEST)/ && Severity == 6"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherFieldString(b *testing.B) {
	b.StopTimer()
	s := "Fields[foo] == 'bar' && Severity == 6"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherFieldNumeric(b *testing.B) {
	b.StopTimer()
	s := "Fields[number] == 64 && Severity == 6"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherFieldNonExistence(b *testing.B) {
	b.StopTimer()
	s := "Fields[missing] == NIL"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherFieldExistence(b *testing.B) {
	b.StopTimer()
	s := "Fields[int] != NIL"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherStartsWith(b *testing.B) {
	b.StopTimer()
	s := "Payload =~ /^Test/"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

func BenchmarkMatcherEndsWith(b *testing.B) {
	b.StopTimer()
	s := "Payload =~ /load$/"
	ms, _ := CreateMatcherSpecification(s)
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ms.Match(msg)
	}
}

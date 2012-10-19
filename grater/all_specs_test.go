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
	//"fmt"
	"github.com/orfjackal/gospec/src/gospec"
	gs "github.com/orfjackal/gospec/src/gospec"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestAllSpecs(t *testing.T) {
	r := gospec.NewRunner()
	r.AddSpec(DecodersSpec)
	r.AddSpec(MessageEqualsSpec)
	gospec.MainGoTest(r, t)
}

func (self *Message) Equals(other interface{}) bool {
	vSelf := reflect.ValueOf(self).Elem()
	vOther := reflect.ValueOf(other).Elem()

	var sField, oField reflect.Value
	var sMap, oMap map[string]interface{}
	for i := 0; i < vSelf.NumField(); i++ {
		sField = vSelf.Field(i)
		oField = vOther.Field(i)
		if sField.Kind() == reflect.Map {
			sMap = sField.Interface().(map[string]interface{})
			oMap = oField.Interface().(map[string]interface{})
			if !reflect.DeepEqual(sMap, oMap) {
				return false
			}
		} else {
			if sField.Interface() != oField.Interface() {
				return false
			}
		}
	}
	return true
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

func MessageEqualsSpec(c gospec.Context) {
	msg0 := getTestMessage()
	msg1Real := *msg0
	msg1 := &msg1Real

	c.Specify("Messages are equal", func() {
		c.Expect(msg0, gs.Equals, msg1)
	})

	c.Specify("Messages w/ diff int values are not equal", func() {
		msg1.Severity--
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff string values are not equal", func() {
		msg1.Payload = "Something completely different"
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})

	c.Specify("Messages w/ diff maps are not equal", func() {
		msg1.Fields = map[string]interface{}{"sna": "foo"}
		c.Expect(msg0, gs.Not(gs.Equals), msg1)
	})
}
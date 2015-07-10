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

/*

Several support structures for use with gospec to ease test comparisons.

*/
package testsupport

import (
	"io/ioutil"
	"log"
	"strings"
	"time"

	. "github.com/mozilla-services/heka/message"
	"github.com/pborman/uuid"
	"github.com/rafrombrc/gospec/src/gospec"
)

var (
	PostTimeout = time.Duration(10 * time.Millisecond)
)

type SimpleT struct{}

func (*SimpleT) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (*SimpleT) Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func StringContains(actual interface{}, criteria interface{}) (match bool,
	pos gospec.Message, neg gospec.Message, err error) {
	toTest := actual.(string)
	critTest := criteria.(string)
	match = strings.Contains(toTest, critTest)
	pos = gospec.Messagef(toTest, "contains "+critTest)
	neg = gospec.Messagef(toTest, "does not contain "+critTest)
	return
}

func StringStartsWith(actual interface{}, criteria interface{}) (match bool,
	pos gospec.Message, neg gospec.Message, err error) {
	actStr := actual.(string)
	critStr := criteria.(string)
	match = actStr[:len(critStr)] == critStr
	pos = gospec.Messagef(actStr, "starts with %s", critStr)
	neg = gospec.Messagef(actStr, "does not start with %s", critStr)
	return
}

func GetTestMessage() *Message {
	hostname := "my.host.name"
	tstr := "Mon Jan 2 15:04:05 -0700 MST 2006"
	ts, _ := time.Parse(tstr, tstr)
	field, _ := NewField("foo", "bar", "")
	msg := &Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(ts.UnixNano())
	msg.SetUuid(uuid.Parse("8e414f01-9d7f-4a48-a5e1-ae92e5954df5"))
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetPayload("Test Payload")
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(43))
	msg.SetHostname(hostname)
	msg.AddField(field)
	return msg
}

// Writes the contents of the provided reader out to a temp file and returns
// the path to the file.
func WriteStringToTmpFile(output string) (path string) {
	file, err := ioutil.TempFile("", "heka-test")
	if err != nil {
		return
	}
	defer file.Close()
	path = file.Name()

	file.WriteString(output)
	return
}

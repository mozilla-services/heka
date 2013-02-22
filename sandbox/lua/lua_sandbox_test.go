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
package lua_test

import "os"
import "time"
import "testing"
import "code.google.com/p/go-uuid/uuid"
import "github.com/mozilla-services/heka/message"
import "github.com/mozilla-services/heka/sandbox"
import "github.com/mozilla-services/heka/sandbox/lua"

func TestCreation(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	b := sb.Memory(sandbox.USAGE_CURRENT)
	if b != 0 {
		t.Errorf("current memory should be 0, using %d", b)
	}
	b = sb.Memory(sandbox.USAGE_MAXIMUM)
	if b != 0 {
		t.Errorf("maximum memory should be 0, using %d", b)
	}
	b = sb.Memory(sandbox.USAGE_LIMIT)
	if b != 32767 {
		t.Errorf("memory limit should be 32767, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_CURRENT)
	if b != 0 {
		t.Errorf("current instructions should be 0, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_MAXIMUM)
	if b != 0 {
		t.Errorf("maximum instructions should be 0, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_LIMIT)
	if b != 1000 {
		t.Errorf("instruction limit should be 1000, using %d", b)
	}
	if sb.LastError() != "" {
		t.Errorf("LastError() should be empty, received: %s", sb.LastError())
	}
	sb.Destroy()
}

func TestCreationTooMuchMemory(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 9000000, 1000)
	if err == nil {
		t.Errorf("Sandbox creation should have failed on maxMem")
		sb.Destroy()
	}
}

func TestCreationTooManyInstructions(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 32767, 1000001)
	if err == nil {
		t.Errorf("Sandbox creation should have failed on maxInst")
		sb.Destroy()
	}
}

func TestInit(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	if sandbox.STATUS_UNKNOWN != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_UNKNOWN, sb.Status())
	}
	err = sb.Init()
	if err != nil {
		t.Errorf("%s", err)
	}
	b := sb.Memory(sandbox.USAGE_CURRENT)
	if b == 0 {
		t.Errorf("current memory should be >0, using %d", b)
	}
	b = sb.Memory(sandbox.USAGE_MAXIMUM)
	if b == 0 {
		t.Errorf("maximum memory should be >0, using %d", b)
	}
	b = sb.Memory(sandbox.USAGE_LIMIT)
	if b != 32767 {
		t.Errorf("memory limit should be 32767, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_CURRENT)
	if b != 4 {
		t.Errorf("current instructions should be 4, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_MAXIMUM)
	if b != 4 {
		t.Errorf("maximum instructions should be 4, using %d", b)
	}
	b = sb.Instructions(sandbox.USAGE_LIMIT)
	if b != 1000 {
		t.Errorf("instruction limit should be 1000, using %d", b)
	}
	if sandbox.STATUS_RUNNING != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_RUNNING, sb.Status())
	}
	sb.Destroy()
}

func TestFailedInit(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/missing.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init()
	if err == nil {
		t.Errorf("Init() should have failed on a missing file")
	}
	if sandbox.STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_TERMINATED, sb.Status())
	}
	s := "init() -> cannot open ./testsupport/missing.lua: No such file or directory"
	if sb.LastError() != s {
		t.Errorf("LastError() should be \"%s\", received: \"%s\"", s, sb.LastError())
	}
	sb.Destroy()
}

func TestMissingProcessMessage(t *testing.T) {
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init()
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(msg)
	if r == 0 {
		t.Errorf("ProcessMessage() expected: 1, received: %d", r)
	}
	s := "process_message() function was not found"
	if sb.LastError() != s {
		t.Errorf("LastError() should be \"%s\", received: \"%s\"", s, sb.LastError())
	}
	if sandbox.STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_TERMINATED, sb.Status())
	}
	r = sb.ProcessMessage(msg) // try to use the terminated plugin
	if r == 0 {
		t.Errorf("ProcessMessage() expected: 1, received: %d", r)
	}
	sb.Destroy()
}

func TestMissingTimeEvent(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/hello_world.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init()
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.TimerEvent()
	if r == 0 {
		t.Errorf("TimerEvent() expected: 1, received: %d", r)
	}
	if sandbox.STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_TERMINATED, sb.Status())
	}
	r = sb.TimerEvent() // try to use the terminated plugin
	if r == 0 {
		t.Errorf("TimerEvent() expected: 1, received: %d", r)
	}
	sb.Destroy()
}

func getTestMessage() *message.Message {
	hostname, _ := os.Hostname()
	field, _ := message.NewField("foo", "bar", message.Field_RAW)
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(time.Now().UnixNano())
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)

	data := []byte("data")
	field1, _ := message.NewField("bytes", data, message.Field_RAW)
	field2, _ := message.NewField("int", int64(999), message.Field_RAW)
	field2.AddValue(int64(1024))
	field3, _ := message.NewField("double", float64(99.9), message.Field_RAW)
	field4, _ := message.NewField("bool", true, message.Field_RAW)
	field5, _ := message.NewField("foo", "alternate", message.Field_RAW)
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)
	msg.AddField(field4)
	msg.AddField(field5)
	return msg
}

func TestAPIErrors(t *testing.T) {
	msg := getTestMessage()
	tests := []string{"send_message() no arg",
		"send_message() incorrect arg type",
		"send_message() incorrect number of args",
		"print() no arg",
		"out of memory",
		"out of instructions",
		"operation on a nil",
		"invalid return",
		"no return"}

	msgs := []string{"process_message() -> send_message() incorrect number of arguments",
		"process_message() -> send_message() argument must be a string",
		"process_message() -> send_message() incorrect number of arguments",
		"process_message() -> print() must have at least one argument",
		"process_message() -> not enough memory",
		"process_message() -> instruction_limit exceeded",
		"process_message() -> ./testsupport/errors.lua:22: attempt to perform arithmetic on global 'x' (a nil value)",
		"process_message() must return a single numeric value",
		"process_message() must return a single numeric value"}

	for i, v := range tests {
		sb, err := lua.CreateLuaSandbox("./testsupport/errors.lua", 32767, 1000)
		if err != nil {
			t.Errorf("%s", err)
		}
		err = sb.Init()
		if err != nil {
			t.Errorf("%s", err)
		}
		msg.SetPayload(v)
		r := sb.ProcessMessage(msg)
		if r != 1 || sandbox.STATUS_TERMINATED != sb.Status() {
			t.Errorf("test: %s status should be %d, received %d",
				v, sandbox.STATUS_TERMINATED, sb.Status())
		}
		s := sb.LastError()
		if s != msgs[i] {
			t.Errorf("test: %s error should be \"%s\", received \"%s\"",
				v, msgs[i], s)
		}
		sb.Destroy()
	}
}

func TestTimerEvent(t *testing.T) {
	sb, err := lua.CreateLuaSandbox("./testsupport/errors.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init()
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.TimerEvent()
	if r != 0 || sandbox.STATUS_RUNNING != sb.Status() {
		t.Errorf("status should be %d, received %d",
			sandbox.STATUS_RUNNING, sb.Status())
	}
	s := sb.LastError()
	if s != "" {
		t.Errorf("there should be no error; received \"%s\"", s)
	}
	sb.Destroy()
}

func TestReadMessage(t *testing.T) {
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox("./testsupport/read_message.lua", 32767, 1000)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init()
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(msg)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d", r)
	}
	r = sb.TimerEvent()
	if r != 0 {
		t.Errorf("read_message should return nil in timer_event")
	}
	sb.Destroy()
}

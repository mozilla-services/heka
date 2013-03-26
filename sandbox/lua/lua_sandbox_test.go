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
import . "github.com/mozilla-services/heka/sandbox"
import "github.com/mozilla-services/heka/sandbox/lua"
import "io/ioutil"
import "bytes"

func TestCreation(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	b := sb.Usage(TYPE_MEMORY, STAT_CURRENT)
	if b != 0 {
		t.Errorf("current memory should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_MEMORY, STAT_MAXIMUM)
	if b != 0 {
		t.Errorf("maximum memory should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_MEMORY, STAT_LIMIT)
	if b != sbc.MemoryLimit {
		t.Errorf("memory limit should be %d, using %d", sbc.MemoryLimit, b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_CURRENT)
	if b != 0 {
		t.Errorf("current instructions should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_MAXIMUM)
	if b != 0 {
		t.Errorf("maximum instructions should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_LIMIT)
	if b != sbc.InstructionLimit {
		t.Errorf("instruction limit should be %d, using %d", sbc.InstructionLimit, b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_CURRENT)
	if b != 0 {
		t.Errorf("current output should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_MAXIMUM)
	if b != 0 {
		t.Errorf("maximum output should be 0, using %d", b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_LIMIT)
	if b != sbc.OutputLimit {
		t.Errorf("output limit should be %d, using %d", sbc.OutputLimit, b)
	}
	b = sb.Usage(TYPE_OUTPUT, 99)
	if b != 0 {
		t.Errorf("invalid index should return 0, received %d", b)
	}
	if sb.LastError() != "" {
		t.Errorf("LastError() should be empty, received: %s", sb.LastError())
	}
	sb.Destroy("")
}

func TestCreationTooMuchMemory(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 9000000
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err == nil {
		t.Errorf("Sandbox creation should have failed on MemoryLimit")
		sb.Destroy("")
	}
}

func TestCreationTooManyInstructions(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000001
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err == nil {
		t.Errorf("Sandbox creation should have failed on InstructionLimit")
		sb.Destroy("")
	}
}

func TestInit(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	if STATUS_UNKNOWN != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_UNKNOWN, sb.Status())
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	b := sb.Usage(TYPE_MEMORY, STAT_CURRENT)
	if b == 0 {
		t.Errorf("current memory should be >0, using %d", b)
	}
	b = sb.Usage(TYPE_MEMORY, STAT_MAXIMUM)
	if b == 0 {
		t.Errorf("maximum memory should be >0, using %d", b)
	}
	b = sb.Usage(TYPE_MEMORY, STAT_LIMIT)
	if b != sbc.MemoryLimit {
		t.Errorf("memory limit should be %d, using %d", sbc.MemoryLimit, b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_CURRENT)
	if b != 9 {
		t.Errorf("current instructions should be 9, using %d", b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_MAXIMUM)
	if b != 9 {
		t.Errorf("maximum instructions should be 9, using %d", b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_LIMIT)
	if b != sbc.InstructionLimit {
		t.Errorf("instruction limit should be %d, using %d", sbc.InstructionLimit, b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_CURRENT)
	if b != 12 {
		t.Errorf("current output should be 12, using %d", b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_MAXIMUM)
	if b != 12 {
		t.Errorf("maximum output should be 12, using %d", b)
	}
	b = sb.Usage(TYPE_OUTPUT, STAT_LIMIT)
	if b != sbc.OutputLimit {
		t.Errorf("output limit should be %d, using %d", sbc.OutputLimit, b)
	}
	if STATUS_RUNNING != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_RUNNING, sb.Status())
	}
	sb.Destroy("")
}

func TestFailedInit(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/missing.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err == nil {
		t.Errorf("Init() should have failed on a missing file")
	}
	if STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_TERMINATED, sb.Status())
	}
	s := "cannot open ./testsupport/missing.lua: No such file or directory"
	if sb.LastError() != s {
		t.Errorf("LastError() should be \"%s\", received: \"%s\"", s, sb.LastError())
	}
	sb.Destroy("")
}

func TestMissingProcessMessage(t *testing.T) {
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(msg, captures)
	if r == 0 {
		t.Errorf("ProcessMessage() expected: 1, received: %d", r)
	}
	s := "process_message() function was not found"
	if sb.LastError() != s {
		t.Errorf("LastError() should be \"%s\", received: \"%s\"", s, sb.LastError())
	}
	if STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_TERMINATED, sb.Status())
	}
	r = sb.ProcessMessage(msg, captures) // try to use the terminated plugin
	if r == 0 {
		t.Errorf("ProcessMessage() expected: 1, received: %d", r)
	}
	sb.Destroy("")
}

func TestMissingTimeEvent(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/hello_world.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.TimerEvent(time.Now().UnixNano())
	if r == 0 {
		t.Errorf("TimerEvent() expected: 1, received: %d", r)
	}
	if STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_TERMINATED, sb.Status())
	}
	r = sb.TimerEvent(time.Now().UnixNano()) // try to use the terminated plugin
	if r == 0 {
		t.Errorf("TimerEvent() expected: 1, received: %d", r)
	}
	sb.Destroy("")
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
	tests := []string{
		"inject_message() incorrect number of args",
		"output() no arg",
		"out of memory",
		"out of instructions",
		"operation on a nil",
		"invalid return",
		"no return",
		"read_message() incorrect number of args",
		"read_message() incorrect field name type",
		"read_message() incorrect field index type",
		"read_message() incorrect array index type",
		"read_message() negative field index",
		"read_message() negative array index",
		"output limit exceeded"}

	msgs := []string{
		"process_message() inject_message() takes no arguments",
		"process_message() output() must have at least one argument",
		"process_message() not enough memory",
		"process_message() instruction_limit exceeded",
		"process_message() ./testsupport/errors.lua:18: attempt to perform arithmetic on global 'x' (a nil value)",
		"process_message() must return a single numeric value",
		"process_message() must return a single numeric value",
		"process_message() read_message() incorrect number of arguments",
		"process_message() ./testsupport/errors.lua:26: bad argument #1 to 'read_message' (string expected, got nil)",
		"process_message() ./testsupport/errors.lua:28: bad argument #2 to 'read_message' (number expected, got nil)",
		"process_message() ./testsupport/errors.lua:30: bad argument #3 to 'read_message' (number expected, got nil)",
		"process_message() read_message() field index must be >= 0",
		"process_message() read_message() array index must be >= 0",
		"process_message() output_limit exceeded"}

	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/errors.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 128
	for i, v := range tests {
		sb, err := lua.CreateLuaSandbox(&sbc)
		if err != nil {
			t.Errorf("%s", err)
		}
		err = sb.Init("")
		if err != nil {
			t.Errorf("%s", err)
		}
		msg.SetPayload(v)
		r := sb.ProcessMessage(msg, captures)
		if r != 1 || STATUS_TERMINATED != sb.Status() {
			t.Errorf("test: %s status should be %d, received %d",
				v, STATUS_TERMINATED, sb.Status())
		}
		s := sb.LastError()
		if s != msgs[i] {
			t.Errorf("test: %s error should be \"%s\", received \"%s\"",
				v, msgs[i], s)
		}
		sb.Destroy("")
	}
}

func TestTimerEvent(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/errors.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.TimerEvent(time.Now().UnixNano())
	if r != 0 || STATUS_RUNNING != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_RUNNING, sb.Status())
	}
	s := sb.LastError()
	if s != "" {
		t.Errorf("there should be no error; received \"%s\"", s)
	}
	sb.Destroy("")
}

func TestReadMessage(t *testing.T) {
	var sbc SandboxConfig
	captures := map[string]string{"exists": "found"}
	sbc.ScriptFilename = "./testsupport/read_message.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(msg, captures)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d", r)
	}
	r = sb.TimerEvent(time.Now().UnixNano())
	if r != 0 {
		t.Errorf("read_message should return nil in timer_event")
	}
	sb.Destroy("")
}

func TestPreserve(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	output := "/tmp/serialize.lua.data"
	saved := "./testsupport/serialize.lua.data"
	err = sb.Destroy("/tmp/serialize.lua.data")
	if err != nil {
		t.Errorf("%s", err)
	} else {
		o, err := ioutil.ReadFile(output)
		if err != nil {
			t.Errorf("%s", err)
		}
		s, err := ioutil.ReadFile(saved)
		if err != nil {
			t.Errorf("%s", err)
		}
		if 0 != bytes.Compare(o, s) {
			t.Errorf("The preserved data does not match")
		}
	}
}

func TestRestore(t *testing.T) {
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/simple_count.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("./testsupport/simple_count.lua.data")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.InjectMessage(func(s string) {
		if s != "11" {
			t.Errorf("State was not restored")
		}
	})
	r := sb.ProcessMessage(msg, captures)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d", r)
	}
	sb.Destroy("")
}

func TestRestoreMissingData(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/simple_count.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("./testsupport/missing.data")
	if err == nil {
		t.Errorf("Init should fail on data load error")
	} else {
		expect := "Init() restore_global_data cannot open ./testsupport/missing.data: No such file or directory"
		if err.Error() != expect {
			t.Errorf("expected '%s' got '%s'", expect, err)
		}
	}
	sb.Destroy("")
}

func TestPreserveFailure(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize_failure.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	output := "/tmp/serialize_failure.lua.data"
	err = sb.Destroy(output)
	if err == nil {
		t.Errorf("The key of type 'function' should have failed")
	} else {
		expect := "Destroy() serialize_data cannot preserve type 'function'"
		if err.Error() != expect {
			t.Errorf("expected '%s' got '%s'", expect, err)
		}
	}
	_, err = os.Stat(output)
	if err == nil {
		t.Errorf("The output file should be removed on failure")
	}
}

func TestPreserveFailureNoGlobal(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize_noglobal.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	output := "/tmp/serialize_noglobal.lua.data"
	err = sb.Destroy(output)
	if err == nil {
		t.Errorf("The global table should be in accessible")
	} else {
		expect := "Destroy() preserve_global_data cannot access the global table"
		if err.Error() != expect {
			t.Errorf("expected '%s' got '%s'", expect, err)
		}
	}
}

func BenchmarkSandboxCreateInitDestroy(b *testing.B) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	for i := 0; i < b.N; i++ {
		sb, _ := lua.CreateLuaSandbox(&sbc)
		sb.Init("")
		sb.Destroy("")
	}
}

func BenchmarkSandboxCreateInitDestroyRestore(b *testing.B) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	for i := 0; i < b.N; i++ {
		sb, _ := lua.CreateLuaSandbox(&sbc)
		sb.Init("./testsupport/serialize.lua")
		sb.Destroy("")
	}
}

func BenchmarkSandboxCreateInitDestroyPreserve(b *testing.B) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/serialize.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	for i := 0; i < b.N; i++ {
		sb, _ := lua.CreateLuaSandbox(&sbc)
		sb.Init("")
		sb.Destroy("/tmp/serialize.lua.data")
	}
}

func BenchmarkSandboxProcessMessageCounter(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg, captures)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageString(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/readstring.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg, captures)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageInt(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/readint.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg, captures)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageField(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	var captures map[string]string
	sbc.ScriptFilename = "./testsupport/readfield.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg, captures)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageCapture(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	captures := map[string]string{"exists": "found"}
	sbc.ScriptFilename = "./testsupport/readcapture.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg, captures)
	}
	sb.Destroy("")
}

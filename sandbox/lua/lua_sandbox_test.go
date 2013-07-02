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
	r := sb.ProcessMessage(msg)
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
	r = sb.ProcessMessage(msg) // try to use the terminated plugin
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
	field, _ := message.NewField("foo", "bar", "")
	msg := &message.Message{}
	msg.SetType("TEST")
	msg.SetTimestamp(5123456789)
	msg.SetPid(9283)
	msg.SetUuid(uuid.NewRandom())
	msg.SetLogger("GoSpec")
	msg.SetSeverity(int32(6))
	msg.SetEnvVersion("0.8")
	msg.SetPid(int32(os.Getpid()))
	msg.SetHostname(hostname)
	msg.AddField(field)

	data := []byte("data")
	field1, _ := message.NewField("bytes", data, "")
	field2, _ := message.NewField("int", int64(999), "")
	field2.AddValue(int64(1024))
	field3, _ := message.NewField("double", float64(99.9), "")
	field4, _ := message.NewField("bool", true, "")
	field5, _ := message.NewField("foo", "alternate", "")
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
		"read_message() negative field index",
		"read_message() negative array index",
		"output limit exceeded"}

	msgs := []string{
		"process_message() ./testsupport/errors.lua:11: inject_message() takes a maximum of 2 arguments",
		"process_message() ./testsupport/errors.lua:13: output() must have at least one argument",
		"process_message() not enough memory",
		"process_message() instruction_limit exceeded",
		"process_message() ./testsupport/errors.lua:22: attempt to perform arithmetic on global 'x' (a nil value)",
		"process_message() must return a single numeric value",
		"process_message() must return a single numeric value",
		"process_message() ./testsupport/errors.lua:28: read_message() incorrect number of arguments",
		"process_message() ./testsupport/errors.lua:30: bad argument #1 to 'read_message' (string expected, got nil)",
		"process_message() ./testsupport/errors.lua:32: bad argument #2 to 'read_message' (field index must be >= 0)",
		"process_message() ./testsupport/errors.lua:34: bad argument #3 to 'read_message' (array index must be >= 0)",
		"process_message() ./testsupport/errors.lua:37: output_limit exceeded"}

	var sbc SandboxConfig
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
		r := sb.ProcessMessage(msg)
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
	r := sb.ProcessMessage(msg)
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
	sbc.MemoryLimit = 64000
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
	sb.InjectMessage(func(p, pt, pn string) int {
		if p != "11" {
			t.Errorf("State was not restored")
		}
		return 0
	})
	r := sb.ProcessMessage(msg)
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

func TestFailedMessageInjection(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/loop.lua"
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
	sb.InjectMessage(func(p, pt, pn string) int {
		return 1
	})
	r := sb.ProcessMessage(msg)
	if r != 1 {
		t.Errorf("ProcessMessage should return 1, received %d", r)
	}
	if STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_TERMINATED, sb.Status())
	}
	s := sb.LastError()
	errMsg := "process_message() ./testsupport/loop.lua:7: inject_message() exceeded MaxMsgLoops"
	if s != errMsg {
		t.Errorf("error should be \"%s\", received \"%s\"", errMsg, s)
	}
	sb.Destroy("")
}

func TestCircularBufferErrors(t *testing.T) {
	msg := getTestMessage()
	tests := []string{
		"new() incorrect # args",
		"new() non numeric row",
		"new() 1 row",
		"new() non numeric column",
		"new() zero column",
		"new() non numeric seconds_per_row",
		"new() zero seconds_per_row",
		"new() > hour seconds_per_row",
		"new() too much memory",
		"set() out of range column",
		"set() zero column",
		"set() non numeric column",
		"set() non numeric time",
		"get() invalid object",
		"set() non numeric value",
		"set() incorrect # args",
		"add() incorrect # args",
		"get() incorrect # args",
		"compute() incorrect # args",
		"compute() incorrect function",
		"compute() incorrect column",
		"compute() start > end"}

	msgs := []string{
		"process_message() ./testsupport/circular_buffer_errors.lua:9: bad argument #-1 to 'new' (incorrect number of arguments)",
		"process_message() ./testsupport/circular_buffer_errors.lua:11: bad argument #1 to 'new' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:13: bad argument #1 to 'new' (rows must be > 1)",
		"process_message() ./testsupport/circular_buffer_errors.lua:15: bad argument #2 to 'new' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:17: bad argument #2 to 'new' (columns must be > 0)",
		"process_message() ./testsupport/circular_buffer_errors.lua:19: bad argument #3 to 'new' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:21: bad argument #3 to 'new' (seconds_per_row is out of range)",
		"process_message() ./testsupport/circular_buffer_errors.lua:23: bad argument #3 to 'new' (seconds_per_row is out of range)",
		"process_message() not enough memory",
		"process_message() ./testsupport/circular_buffer_errors.lua:28: bad argument #2 to 'set' (column out of range)",
		"process_message() ./testsupport/circular_buffer_errors.lua:31: bad argument #2 to 'set' (column out of range)",
		"process_message() ./testsupport/circular_buffer_errors.lua:34: bad argument #2 to 'set' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:37: bad argument #1 to 'set' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:41: bad argument #1 to 'get' (Heka.circular_buffer expected, got number)",
		"process_message() ./testsupport/circular_buffer_errors.lua:44: bad argument #3 to 'set' (number expected, got nil)",
		"process_message() ./testsupport/circular_buffer_errors.lua:47: bad argument #-1 to 'set' (incorrect number of arguments)",
		"process_message() ./testsupport/circular_buffer_errors.lua:50: bad argument #-1 to 'add' (incorrect number of arguments)",
		"process_message() ./testsupport/circular_buffer_errors.lua:53: bad argument #-1 to 'get' (incorrect number of arguments)",
		"process_message() ./testsupport/circular_buffer_errors.lua:56: bad argument #-1 to 'compute' (incorrect number of arguments)",
		"process_message() ./testsupport/circular_buffer_errors.lua:59: bad argument #1 to 'compute' (invalid option 'func')",
		"process_message() ./testsupport/circular_buffer_errors.lua:62: bad argument #2 to 'compute' (column out of range)",
		"process_message() ./testsupport/circular_buffer_errors.lua:65: bad argument #4 to 'compute' (end must be >= start)",
	}

	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/circular_buffer_errors.lua"
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
		r := sb.ProcessMessage(msg)
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

func TestCircularBuffer(t *testing.T) {
	msg := getTestMessage()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/circular_buffer.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 32767
	payload_type := "cbuf"
	payload_name := "Method tests"

	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	output := []string{
		"{\"time\":0,\"rows\":3,\"columns\":3,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Add_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Set_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Get_column\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n0\t0\t0\n0\t0\t0\n0\t0\t0\n",
		"{\"time\":0,\"rows\":3,\"columns\":3,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Add_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Set_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Get_column\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n1\t1\t1\n2\t1\t2\n3\t1\t3\n",
		"{\"time\":2,\"rows\":3,\"columns\":3,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Add_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Set_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Get_column\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n3\t1\t3\n0\t0\t0\n1\t1\t1\n",
		"{\"time\":8,\"rows\":3,\"columns\":3,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Add_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Set_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Get_column\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n0\t0\t0\n0\t0\t0\n1\t1\t1\n"}
	cnt := 0
	sb.InjectMessage(func(p, pt, pn string) int {
		if p != output[cnt] {
			t.Errorf("cnt: %d buffer should be \"%s\", received \"%s\"", cnt,
				output[cnt], p)
		}
		if pt != payload_type {
			t.Errorf("cnt: %d type should be \"%s\", received \"%s\"", cnt, payload_type, pt)
		}
		if pn != "Method tests" {
			t.Errorf("cnt: %d name should be \"%s\", received \"%s\"", cnt, payload_name, pn)
		}
		cnt++
		return 0
	})
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.TimerEvent(0)
	msg.SetTimestamp(0)
	r := sb.ProcessMessage(msg)
	if r != 0 {
		t.Errorf("ProcessMessage failed: %s", sb.LastError())
	}
	msg.SetTimestamp(1e9)
	sb.ProcessMessage(msg)
	sb.ProcessMessage(msg)
	msg.SetTimestamp(2e9)
	sb.ProcessMessage(msg)
	sb.ProcessMessage(msg)
	sb.ProcessMessage(msg)
	sb.TimerEvent(0)

	msg.SetTimestamp(4e9)
	sb.ProcessMessage(msg)
	sb.TimerEvent(0)

	msg.SetTimestamp(10e9)
	r = sb.ProcessMessage(msg)
	if r != 0 {
		t.Errorf("ProcessMessage failed: %s", sb.LastError())
	}
	sb.TimerEvent(0)
	sb.TimerEvent(1)
	r = sb.TimerEvent(3)
	if r != 0 {
		t.Errorf("TimerEvent failed: %s", sb.LastError())
	}
	sb.Destroy("/tmp/circular_buffer.lua.data")
}

func TestCircularBufferRestore(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/circular_buffer.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 32767
	payload_type := "cbuf"

	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	output := []string{
		"{\"time\":8,\"rows\":3,\"columns\":3,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Add_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Set_column\",\"unit\":\"count\",\"aggregation\":\"sum\"},{\"name\":\"Get_column\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n0\t0\t0\n0\t0\t0\n3\t1\t3\n",
		"{\"time\":0,\"rows\":2,\"columns\":1,\"seconds_per_row\":1,\"column_info\":[{\"name\":\"Header_1\",\"unit\":\"count\",\"aggregation\":\"sum\"}]}\n7.1\n1000000\n"}
	cnt := 0
	sb.InjectMessage(func(p, pt, pn string) int {
		if p != output[cnt] {
			t.Errorf("cnt: %d buffer should be \"%s\", received \"%s\"", cnt,
				output[cnt], p)
		}
		if pt != payload_type {
			t.Errorf("cnt: %d type should be \"%s\", received \"%s\"", cnt, payload_type, pt)
		}
		if cnt == 1 && len(pn) != 0 {
			t.Errorf("cnt: %d name should be empty, received \"%s\"", cnt, pn)
		}
		cnt++
		return 0
	})
	err = sb.Init("./testsupport/circular_buffer.lua.data")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.TimerEvent(0)
	sb.TimerEvent(2)
	sb.Destroy("")
}

func TestInjectMessage(t *testing.T) {
	var sbc SandboxConfig
	tests := []string{
		"lua types",
		"cloudwatch metric",
		"external reference",
		"array only",
		"private keys",
		"table name",
		"global table",
		"special characters",
	}

	outputs := []string{
		`{"table":{"value":1}}
1.2 string nil true false`,
		`{"table":{"StatisticValues":[{"Minimum":0,"SampleCount":0,"Sum":0,"Maximum":0},{"Minimum":0,"SampleCount":0,"Sum":0,"Maximum":0}],"Dimensions":[{"Name":"d1","Value":"v1"},{"Name":"d2","Value":"v2"}],"MetricName":"example","Timestamp":0,"Value":0,"Unit":"s"}}
`,
		`{"table":{"a":{"y":2,"x":1}}}
`,
		`{"table":[1,2,3]}
`,
		`{"table":{"x":1}}
`,
		`{"array":[1,2,3]}
`,
		`{"table":{}}
`,
		`{"table":{"special\tcharacters":"\"\t\r\n\b\f\\\/"}}
`,
	}
	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	msg := getTestMessage()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	cnt := 0
	sb.InjectMessage(func(p, pt, pn string) int {
		if p != outputs[cnt] {
			t.Errorf("Output is incorrect, expected: \"%s\" received: \"%s\"", outputs[cnt], p)
		}
		cnt++
		return 0
	})

	for _, v := range tests {
		msg.SetPayload(v)
		r := sb.ProcessMessage(msg)
		if r != 0 {
			t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
		}
	}
	sb.Destroy("")
}

func TestInjectMessageError(t *testing.T) {
	var sbc SandboxConfig
	tests := []string{
		"error internal reference",
		"error circular reference",
		"error escape overflow",
	}
	errors := []string{
		"process_message() ./testsupport/inject_message.lua:46: table contains an internal or circular reference",
		"process_message() ./testsupport/inject_message.lua:51: table contains an internal or circular reference",
		"process_message() not enough memory",
	}

	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	msg := getTestMessage()
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
		r := sb.ProcessMessage(msg)
		if r != 1 {
			t.Errorf("ProcessMessage should return 1, received %d", r)
		} else {
			if sb.LastError() != errors[i] {
				t.Errorf("Expected: \"%s\" received: \"%s\"", errors[i], sb.LastError())
			}
		}
		sb.Destroy("")
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
	sbc.ScriptFilename = "./testsupport/lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageString(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readstring.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageInt(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readint.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageField(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readfield.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxOutputLuaTypes(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	msg.SetPayload("lua types")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxOutputTable(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	msg := getTestMessage()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	msg.SetPayload("cloudwatch metric")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(msg)
	}
	sb.Destroy("")
}

func BenchmarkSandboxOutputCbuf(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 64512
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.TimerEvent(0)
	}
	sb.Destroy("")
}

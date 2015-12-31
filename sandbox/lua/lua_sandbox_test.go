/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012-2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package lua_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	. "github.com/mozilla-services/heka/sandbox"
	"github.com/mozilla-services/heka/sandbox/lua"
	"github.com/pborman/uuid"
)

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
	if b != 6 {
		t.Errorf("current instructions should be 9, using %d", b)
	}
	b = sb.Usage(TYPE_INSTRUCTIONS, STAT_MAXIMUM)
	if b != 6 {
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
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(pack)
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
	r = sb.ProcessMessage(pack) // try to use the terminated plugin
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

	var emptyByte []byte
	data := []byte("data")
	field1, _ := message.NewField("bytes", data, "")
	field2, _ := message.NewField("int", int64(999), "")
	field2.AddValue(int64(1024))
	field3, _ := message.NewField("double", float64(99.9), "")
	field4, _ := message.NewField("bool", true, "")
	field5, _ := message.NewField("foo", "alternate", "")
	field6, _ := message.NewField("false", false, "")
	field7, _ := message.NewField("empty_bytes", emptyByte, "")
	msg.AddField(field1)
	msg.AddField(field2)
	msg.AddField(field3)
	msg.AddField(field4)
	msg.AddField(field5)
	msg.AddField(field6)
	msg.AddField(field7)
	return msg
}

func getTestPack() *pipeline.PipelinePack {
	pack := pipeline.NewPipelinePack(nil)
	pack.Message = getTestMessage()
	return pack
}

func TestAPIErrors(t *testing.T) {
	pack := getTestPack()
	tests := []string{
		"require unknown",
		"add_to_payload() no arg",
		"out of memory",
		"out of instructions",
		"operation on a nil",
		"invalid return",
		"no return",
		"read_message() incorrect number of args",
		"read_message() incorrect field name type",
		"read_message() negative field index",
		"read_message() negative array index",
		"output limit exceeded",
		"read_config() must have a single argument",
		"read_next_field() takes no arguments",
		"write_message() should not exist",
		"invalid error message",
	}
	msgs := []string{
		"process_message() ./testsupport/errors.lua:11: module 'unknown' not found:\n\tno file 'unknown.lua'\n\tno file 'unknown.so'",
		"process_message() ./testsupport/errors.lua:13: bad argument #0 to 'add_to_payload' (must have at least one argument)",
		"process_message() not enough memory",
		"process_message() instruction_limit exceeded",
		"process_message() ./testsupport/errors.lua:22: attempt to perform arithmetic on global 'x' (a nil value)",
		"process_message() must return a numeric status code",
		"process_message() must return a numeric status code",
		"process_message() ./testsupport/errors.lua:28: read_message() incorrect number of arguments",
		"process_message() ./testsupport/errors.lua:30: bad argument #1 to 'read_message' (string expected, got nil)",
		"process_message() ./testsupport/errors.lua:32: bad argument #2 to 'read_message' (field index must be >= 0)",
		"process_message() ./testsupport/errors.lua:34: bad argument #3 to 'read_message' (array index must be >= 0)",
		"process_message() ./testsupport/errors.lua:37: output_limit exceeded",
		"process_message() ./testsupport/errors.lua:40: read_config() must have a single argument",
		"process_message() ./testsupport/errors.lua:42: read_next_field() takes no arguments",
		"process_message() ./testsupport/errors.lua:44: attempt to call global 'write_message' (a nil value)",
		"process_message() must return a nil or string error message",
	}

	if runtime.GOOS == "windows" {
		msgs[0] = "process_message() ./testsupport/errors.lua:11: module 'unknown' not found:\n\tno file 'unknown.lua'\n\tno file 'unknown.dll'"
	}

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
		pack.Message.SetPayload(v)
		r := sb.ProcessMessage(pack)
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

func TestWriteMessageErrors(t *testing.T) {
	pack := getTestPack()
	// NewPipelineConfig sets up Globals for error logging
	pipeline.NewPipelineConfig(nil)

	tests := []string{
		"too few parameters",
		"too many parameters",
		"Unknown field name",
		"Missing fields specifier",
		"Missing closing bracket",
		"Out of range field index",
		"Negative field index",
		"Negative array index",
		"nil field",
		"empty uuid",
		"invalid uuid",
		"empty timestamp",
		"invalid timestamp",
		"bool severity",
		"double hostname",
		"invalid field type",
		"out of range field index deletion",
		"out of range field array index deletion",
	}
	msgs := []string{
		"process_message() ./testsupport/write_message_errors.lua:11: write_message() incorrect number of arguments",
		"process_message() ./testsupport/write_message_errors.lua:13: write_message() incorrect number of arguments",
		"process_message() ./testsupport/write_message_errors.lua:15: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:17: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:19: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:21: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:23: bad argument #4 to 'write_message' (field index must be >= 0)",
		"process_message() ./testsupport/write_message_errors.lua:25: bad argument #5 to 'write_message' (array index must be >= 0)",
		"process_message() ./testsupport/write_message_errors.lua:27: bad argument #1 to 'write_message' (string expected, got nil)",
		"process_message() ./testsupport/write_message_errors.lua:29: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:31: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:33: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:35: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:37: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:39: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:41: write_message() only accepts numeric, string, or boolean field values, or nil to delete",
		"process_message() ./testsupport/write_message_errors.lua:43: write_message() failed",
		"process_message() ./testsupport/write_message_errors.lua:45: write_message() failed",
	}

	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/write_message_errors.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 128
	sbc.PluginType = "decoder"
	for i, v := range tests {
		sb, err := lua.CreateLuaSandbox(&sbc)
		if err != nil {
			t.Errorf("%s", err)
		}
		err = sb.Init("")
		if err != nil {
			t.Errorf("%s", err)
		}
		pack.Message.SetPayload(v)
		r := sb.ProcessMessage(pack)
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
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	pack.MsgBytes = []byte("rawdata")
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d", r)
	}
	r = sb.TimerEvent(time.Now().UnixNano())
	if r != 0 {
		t.Errorf("read_message should return nil in timer_event")
	}
	sb.Destroy("")
}

func TestReadRaw(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/read_raw.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d last error: %s", r,
			sb.LastError())
	}
	sb.Destroy("")
}

func TestWriteMessage(t *testing.T) {
	pipeline.NewPipelineConfig(nil) // Set up globals.
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/field_scribble.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.PluginType = "decoder"
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d", r)
	}
	if pack.Message.GetType() != "MyType" {
		t.Error("Type not set")
	}
	if pack.Message.GetLogger() != "MyLogger" {
		t.Error("Logger not set")
	}
	packTime := time.Unix(0, pack.Message.GetTimestamp())
	cmpTime := time.Unix(0, 1385968914904958136)
	d, _ := time.ParseDuration("500ns")
	packTime = packTime.Round(d)
	cmpTime = cmpTime.Round(d)
	if !packTime.Equal(cmpTime) {
		t.Errorf("Timestamp not set: %d", pack.Message.GetTimestamp())
	}
	if pack.Message.GetPayload() != "MyPayload" {
		t.Error("Payload not set")
	}
	if pack.Message.GetEnvVersion() != "000" {
		t.Error("EnvVersion not set")
	}
	if pack.Message.GetHostname() != "MyHostname" {
		t.Error("Hostname not set")
	}
	if pack.Message.GetSeverity() != 4 {
		t.Error("Severity not set")
	}
	if pack.Message.GetPid() != 12345 {
		t.Error("Pid not set")
	}
	var f []*message.Field
	f = pack.Message.FindAllFields("String")
	if len(f) != 1 || len(f[0].GetValueString()) != 1 || f[0].GetValueString()[0] != "foo" {
		t.Errorf("String field not set")
	}
	f = pack.Message.FindAllFields("Float")
	if len(f) != 1 || len(f[0].GetValueDouble()) != 1 || f[0].GetValueDouble()[0] != 1.2345 {
		t.Error("Float field not set")
	}
	f = pack.Message.FindAllFields("Int")
	if len(f) != 1 {
		t.Error("Int field not set")
	} else {
		if len(f[0].GetValueDouble()) != 2 || f[0].GetValueDouble()[0] != 123 ||
			f[0].GetValueDouble()[1] != 456 {
			t.Error("Int field set incorrectly")
		}
		if f[0].GetRepresentation() != "count" {
			t.Error("Int field representation not set")
		}
	}
	f = pack.Message.FindAllFields("Bool")
	if len(f) != 2 {
		t.Error("Bool fields not set")
	} else {
		if len(f[0].GetValueBool()) != 1 || !f[0].GetValueBool()[0] {
			t.Error("Bool field 0 not set")
		}
		if len(f[1].GetValueBool()) != 1 || f[1].GetValueBool()[0] {
			t.Error("Bool field 1 not set")
		}
	}
	if f = pack.Message.FindAllFields(""); len(f) != 1 {
		t.Error("No-name field not set")
	} else {
		if len(f[0].GetValueString()) != 1 || f[0].GetValueString()[0] != "bad idea" {
			t.Error("No-name field set incorrectly")
		}
	}
	if pack.Message.GetUuidString() != "550d19b9-58c7-49d8-b0dd-b48cd1c5b305" {
		t.Errorf("Uuid not set: %s", pack.Message.GetUuidString())
	}
	if f = pack.Message.FindAllFields("delete"); len(f) != 0 {
		t.Error("'delete' field not deleted")
	}
}

func TestRestore(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/simple_count.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	pack := getTestPack()
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
	r := sb.ProcessMessage(pack)
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
	if err != nil {
		t.Errorf("%s", err)
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
	output := filepath.Join(os.TempDir(), "serialize_failure.lua.data")
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

func TestFailedMessageInjection(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/loop.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.InjectMessage(func(p, pt, pn string) int {
		return 3
	})
	r := sb.ProcessMessage(pack)
	if r != 1 {
		t.Errorf("ProcessMessage should return 1, received %d", r)
	}
	if STATUS_TERMINATED != sb.Status() {
		t.Errorf("status should be %d, received %d",
			STATUS_TERMINATED, sb.Status())
	}
	s := sb.LastError()
	errMsg := "process_message() ./testsupport/loop.lua:6: inject_payload() exceeded MaxMsgLoops"
	if s != errMsg {
		t.Errorf("error should be \"%s\", received \"%s\"", errMsg, s)
	}
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
		"special characters",
		"message field all types",
		"internal reference",
		"round trip",
		"inject raw",
	}
	outputs := []string{
		`{"value":1}1.2 string nil true false`,
		`{"StatisticValues":[{"Minimum":0,"SampleCount":0,"Sum":0,"Maximum":0},{"Minimum":0,"SampleCount":0,"Sum":0,"Maximum":0}],"Dimensions":[{"Name":"d1","Value":"v1"},{"Name":"d2","Value":"v2"}],"MetricName":"example","Timestamp":0,"Value":0,"Unit":"s"}`,
		`{"a":{"y":2,"x":1}}`,
		`[1,2,3]`,
		`{"x":1,"_m":1,"_private":[1,2]}`,
		`{"special\tcharacters":"\"\t\r\n\b\f\\\/"}`,
		"\x10\x80\x94\xeb\xdc\x03\x52\x13\x0a\x06\x6e\x75\x6d\x62\x65\x72\x10\x03\x39\x00\x00\x00\x00\x00\x00\xf0\x3f\x52\x2c\x0a\x07\x6e\x75\x6d\x62\x65\x72\x73\x10\x03\x1a\x05\x63\x6f\x75\x6e\x74\x3a\x18\x00\x00\x00\x00\x00\x00\xf0\x3f\x00\x00\x00\x00\x00\x00\x00\x40\x00\x00\x00\x00\x00\x00\x08\x40\x52\x0e\x0a\x05\x62\x6f\x6f\x6c\x73\x10\x04\x42\x03\x01\x00\x00\x52\x0a\x0a\x04\x62\x6f\x6f\x6c\x10\x04\x40\x01\x52\x10\x0a\x06\x73\x74\x72\x69\x6e\x67\x22\x06\x73\x74\x72\x69\x6e\x67\x52\x15\x0a\x07\x73\x74\x72\x69\x6e\x67\x73\x22\x02\x73\x31\x22\x02\x73\x32\x22\x02\x73\x33",
		`{"y":[2],"x":[1,2,3],"ir":[1,2,3]}`,
		"\x10\x80\x94\xeb\xdc\x03\x52\x1b\x0a\x05\x63\x6f\x75\x6e\x74\x10\x03\x3a\x10\x00\x00\x00\x00\x00\x00\xf0\x3f\x00\x00\x00\x00\x00\x00\xf0\x3f",
		"\x10\x80\x94\xeb\xdc\x03\x52\x1b\x0a\x05\x63\x6f\x75\x6e\x74\x10\x03\x3a\x10\x00\x00\x00\x00\x00\x00\xf0\x3f\x00\x00\x00\x00\x00\x00\xf0\x3f",
	}
	if false { // lua jit values
		outputs[1] = `{"Timestamp":0,"Value":0,"StatisticValues":[{"SampleCount":0,"Sum":0,"Maximum":0,"Minimum":0},{"SampleCount":0,"Sum":0,"Maximum":0,"Minimum":0}],"Unit":"s","MetricName":"example","Dimensions":[{"Name":"d1","Value":"v1"},{"Name":"d2","Value":"v2"}]}`
	}

	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
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
		if len(pt) == 0 { // no type is a Heka protobuf message
			if p[18:] != outputs[cnt] { // ignore the UUID
				t.Errorf("Output is incorrect, expected: \"%x\" received: \"%x\"", outputs[cnt], p[18:])
			}
		} else {
			if p != outputs[cnt] {
				t.Errorf("Output is incorrect, expected: \"%s\" received: \"%s\"", outputs[cnt], p)
			}
		}
		if cnt == 6 {
			msg := new(message.Message)
			err := proto.Unmarshal([]byte(p), msg)
			if err != nil {
				t.Errorf("%s", err)
			}
			if msg.GetTimestamp() != 1e9 {
				t.Errorf("Timestamp expected %d received %d", int(1e9), pack.Message.GetTimestamp())
			}
			if field := msg.FindFirstField("numbers"); field != nil {
				if field.GetRepresentation() != "count" {
					t.Errorf("'numbers' representation expected \"count\" received \"%s\"", field.GetRepresentation())
				}
			} else {
				t.Errorf("'numbers' field not found")
			}
			tests := []string{
				"Timestamp == 1000000000",
				"Fields[number] == 1",
				"Fields[numbers][0][0] == 1 && Fields[numbers][0][1] == 2 && Fields[numbers][0][2] == 3",
				"Fields[string] == 'string'",
				"Fields[strings][0][0] == 's1' && Fields[strings][0][1] == 's2' && Fields[strings][0][2] == 's3'",
				"Fields[bool] == TRUE",
				"Fields[bools][0][0] == TRUE && Fields[bools][0][1] == FALSE && Fields[bools][0][2] == FALSE",
			}
			for _, v := range tests {
				ms, _ := message.CreateMatcherSpecification(v)
				match := ms.Match(msg)
				if !match {
					t.Errorf("Test failed %s", v)
				}
			}
		}
		cnt++
		return 0
	})

	for _, v := range tests {
		pack.Message.SetPayload(v)
		r := sb.ProcessMessage(pack)
		if r != 0 {
			t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
		}
	}
	sb.Destroy("")
	if cnt != len(tests) {
		t.Errorf("InjectMessage was called %d times, expected %d", cnt, len(tests))
	}
}

func TestInjectMessageError(t *testing.T) {
	var sbc SandboxConfig
	tests := []string{
		"error circular reference",
		"error escape overflow",
		"error mis-match field array",
		"error nil field",
		"error nil type arg",
		"error nil name arg",
		"error nil message",
		"error userdata output_limit",
		"error invalid protobuf string",
	}
	errors := []string{
		"process_message() ./testsupport/inject_message.lua:38: Cannot serialise, excessive nesting (1001)",
		"process_message() ./testsupport/inject_message.lua:44: strbuf output_limit exceeded",
		"process_message() ./testsupport/inject_message.lua:50: inject_message() could not encode protobuf - array has mixed types",
		"process_message() ./testsupport/inject_message.lua:53: inject_message() could not encode protobuf - unsupported type: nil",
		"process_message() ./testsupport/inject_message.lua:55: bad argument #1 to 'inject_payload' (string expected, got nil)",
		"process_message() ./testsupport/inject_message.lua:57: bad argument #2 to 'inject_payload' (string expected, got nil)",
		"process_message() ./testsupport/inject_message.lua:59: inject_message() takes a single string or table argument",
		"process_message() ./testsupport/inject_message.lua:62: output_limit exceeded",
		"process_message() ./testsupport/inject_message.lua:69: inject_message() protobuf unmarshal failed",
	}

	sbc.ScriptFilename = "./testsupport/inject_message.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 1000000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 1024
	pack := getTestPack()
	for i, v := range tests {
		sb, err := lua.CreateLuaSandbox(&sbc)
		if i == 8 {
			sb.InjectMessage(func(p, pt, pn string) int {
				msg := new(message.Message)
				err := proto.Unmarshal([]byte(p), msg)
				if err != nil {
					return 1
				}
				return 0
			})
		}
		if err != nil {
			t.Errorf("%s", err)
		}
		err = sb.Init("")
		if err != nil {
			t.Errorf("%s", err)
		}
		pack.Message.SetPayload(v)
		r := sb.ProcessMessage(pack)
		if r != 1 {
			t.Errorf("ProcessMessage test: %s should return 1, received %d", v, r)
		} else {
			if sb.LastError() != errors[i] {
				t.Errorf("Expected: \"%s\" received: \"%s\"", errors[i], sb.LastError())
			}
		}
		sb.Destroy("")
	}
}

func TestLpeg(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/lpeg_csv.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.InjectMessage(func(p, pt, pn string) int {
		expected := `["1","string with spaces","quoted string, with comma and \"quoted\" text"]`
		if p != expected {
			t.Errorf("Output is incorrect, expected: \"%s\" received: \"%s\"", expected, p)
		}
		return 0
	})

	pack.Message.SetPayload("1,string with spaces,\"quoted string, with comma and \"\"quoted\"\" text\"")
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
}

func TestReadConfig(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/read_config.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	sbc.Config = make(map[string]interface{})
	sbc.Config["string"] = "widget"
	sbc.Config["int64"] = int64(99)
	sbc.Config["double"] = 99.123
	sbc.Config["bool"] = true
	sbc.Config["array"] = []int{1, 2, 3}
	sbc.Config["object"] = map[string]string{"item": "test"}
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	sb.Destroy("")
}

func TestCJson(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/cjson.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	pack.Message.SetPayload("[ true, { \"foo\": \"bar\" } ]")
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
}

func TestGraphiteHelpers(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/graphite.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	sbc.Config = make(map[string]interface{})

	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}

	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}

	for i := 0; i < 4; i++ {
		pack := getTestPack()
		pack.Message.SetHostname("localhost")
		pack.Message.SetLogger("GoSpec")

		message.NewIntField(pack.Message, "status", 200, "status")

		message.NewIntField(pack.Message, "request_time", 15*i, "request_time")
		r := sb.ProcessMessage(pack)
		if r != 0 {
			t.Errorf("Graphite returned %s", r)
		}
	}

	injectCount := 0
	sb.InjectMessage(func(payload, payload_type, payload_name string) int {
		graphite_payload := `stats.counters.localhost.nginx.GoSpec.http_200.count 4 0
stats.counters.localhost.nginx.GoSpec.http_200.rate 0.400000 0
stats.timers.localhost.nginx.GoSpec.request_time.count 4 0
stats.timers.localhost.nginx.GoSpec.request_time.count_ps 0.400000 0
stats.timers.localhost.nginx.GoSpec.request_time.lower 0.000000 0
stats.timers.localhost.nginx.GoSpec.request_time.upper 45.000000 0
stats.timers.localhost.nginx.GoSpec.request_time.sum 90.000000 0
stats.timers.localhost.nginx.GoSpec.request_time.mean 22.500000 0
stats.timers.localhost.nginx.GoSpec.request_time.mean_90 22.500000 0
stats.timers.localhost.nginx.GoSpec.request_time.upper_90 45.000000 0
stats.statsd.numStats 2 0
`
		if payload_type != "txt" {
			t.Errorf("Received payload type: %s", payload_type)
		}

		if payload_name != "statmetric" {
			t.Errorf("Received payload name: %s", payload_name)
		}

		if graphite_payload != payload {
			t.Errorf("Received payload: %s", payload)
		}
		injectCount += 1
		return 0
	})

	sb.TimerEvent(200)

	if injectCount > 0 {
		t.Errorf("Looks there was an error during timer_event")
	}

	sb.Destroy("")
}

func TestReadNilConfig(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/read_config_nil.lua"
	sbc.ModuleDirectory = "./modules"
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
	sb.Destroy("")
}

func TestExternalModule(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/require.lua"
	sbc.ModuleDirectory = "./testsupport"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(pack)
	if r != 43 {
		t.Errorf("ProcessMessage should return 43, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
}

func TestReadNextField(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/read_next_field.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
}

func TestAlert(t *testing.T) {
	var sbc SandboxConfig
	tests := []string{
		"alert1\nalert2\nalert3",
		"alert5",
		"alert8",
	}

	sbc.ScriptFilename = "./testsupport/alert.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
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
		if pt != "alert" {
			t.Errorf("Payload type, expected: \"alert\" received: \"%s\"", pt)
		}
		if p != tests[cnt] {
			t.Errorf("Output is incorrect, expected: \"%s\" received: \"%s\"", tests[cnt], p)
		}
		cnt++
		return 0
	})

	for i, _ := range tests {
		pack.Message.SetTimestamp(int64(i))
		r := sb.ProcessMessage(pack)
		if r != 0 {
			t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
		}
	}
	sb.Destroy("")
	if cnt != len(tests) {
		t.Errorf("Executed %d test, expected %d", cnt, len(tests))
	}
}

func TestAnnotation(t *testing.T) {
	var sbc SandboxConfig
	tests := []string{
		"{\"annotations\":[{\"text\":\"anomaly\",\"x\":1000,\"shortText\":\"A\",\"col\":1},{\"text\":\"anomaly2\",\"x\":5000,\"shortText\":\"A\",\"col\":2},{\"text\":\"maintenance\",\"x\":60000,\"shortText\":\"M\",\"col\":1}]}\n",
		"{\"annotations\":[{\"text\":\"maintenance\",\"x\":60000,\"shortText\":\"M\",\"col\":1}]}\n",
		"{\"annotations\":{}}\n",
		"ok",
	}

	sbc.ScriptFilename = "./testsupport/annotation.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 100000
	sbc.InstructionLimit = 1000
	sbc.OutputLimit = 8000
	pack := getTestPack()
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
		if p != tests[cnt] {
			t.Errorf("Output is incorrect, expected: \"%s\" received: \"%s\"", tests[cnt], p)
		}
		cnt++
		return 0
	})

	for i, _ := range tests {
		pack.Message.SetTimestamp(int64(i))
		r := sb.ProcessMessage(pack)
		if r != 0 {
			t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
		}
	}
	sb.Destroy("")
	if cnt != len(tests) {
		t.Errorf("Executed %d test, expected %d", cnt, len(tests))
	}
}

func TestAnomaly(t *testing.T) {
	var sbc SandboxConfig

	sbc.ScriptFilename = "./testsupport/anomaly.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 1e6
	sbc.InstructionLimit = 1e6
	sbc.OutputLimit = 1000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Errorf("%s", err)
	}
	err = sb.Init("")
	if err != nil {
		t.Errorf("%s", err)
	}

	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
}

func TestElasticSearch(t *testing.T) {
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/elasticsearch.lua"
	sbc.ModuleDirectory = "./modules"
	sbc.MemoryLimit = 1e6
	sbc.InstructionLimit = 1e6
	sbc.OutputLimit = 1000
	pack := getTestPack()
	sb, err := lua.CreateLuaSandbox(&sbc)
	if err != nil {
		t.Error(err)
	}
	if err = sb.Init(""); err != nil {
		t.Error(err)
	}
	r := sb.ProcessMessage(pack)
	if r != 0 {
		t.Errorf("ProcessMessage should return 0, received %d %s", r, sb.LastError())
	}
	sb.Destroy("")
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
		sb.Init("./testsupport/serialize.lua.data")
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
	sbc.ScriptFilename = "./testsupport/counter.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageString(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readstring.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageInt(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readint.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
	}
	sb.Destroy("")
}

func BenchmarkSandboxReadMessageField(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/readfield.lua"
	sbc.MemoryLimit = 32767
	sbc.InstructionLimit = 1000
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
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
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	pack.Message.SetPayload("lua types")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
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
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	pack.Message.SetPayload("cloudwatch metric")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
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

func BenchmarkSandboxOutputMessage(b *testing.B) {
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
		sb.TimerEvent(1)
	}
	sb.Destroy("")
}

func BenchmarkSandboxOutputMessageAsJSON(b *testing.B) {
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
		sb.TimerEvent(2)
	}
	sb.Destroy("")
}

func BenchmarkSandboxLpegDecoder(b *testing.B) {
	b.StopTimer()
	var sbc SandboxConfig
	sbc.ScriptFilename = "./testsupport/decoder.lua"
	sbc.MemoryLimit = 1024 * 1024 * 8
	sbc.InstructionLimit = 1e6
	sbc.OutputLimit = 1024 * 63
	pack := getTestPack()
	sb, _ := lua.CreateLuaSandbox(&sbc)
	sb.Init("")
	sb.InjectMessage(func(p, pt, pn string) int {
		return 0
	})
	pack.Message.SetPayload("1376389920 debug id=2321 url=example.com item=1")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		sb.ProcessMessage(pack)
	}
	sb.Destroy("")
}

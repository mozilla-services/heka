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
package lua

/*
#cgo CFLAGS: -std=gnu99 -I ../../../../../../release/external/include
#cgo LDFLAGS: -L../../../../../../bin -lsandbox -lm
#include <stdlib.h>
#include "lua_sandbox.h"
*/
import "C"

import (
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	"log"
	"strings"
	"unsafe"
)

func lookup_field(msg *message.Message, fn string, fi int, ai int) (int,
	unsafe.Pointer, int) {

	var field *message.Field
	if fi != 0 {
		fields := msg.FindAllFields(fn)
		if fi >= len(fields) {
			return 0, unsafe.Pointer(nil), 0
		}
		field = fields[fi]
	} else {
		if field = msg.FindFirstField(fn); field == nil {
			return 0, unsafe.Pointer(nil), 0
		}
	}
	fieldType := int(field.GetValueType())
	switch field.GetValueType() {
	case message.Field_STRING:
		if ai >= len(field.ValueString) {
			return fieldType, unsafe.Pointer(nil), 0
		}
		value := field.ValueString[ai]
		cs := C.CString(value) // freed by the caller
		return fieldType, unsafe.Pointer(cs), len(value)
	case message.Field_BYTES:
		if ai >= len(field.ValueBytes) {
			return fieldType, unsafe.Pointer(nil), 0
		}
		value := field.ValueBytes[ai]
		return fieldType, unsafe.Pointer(&field.ValueBytes[ai][0]), len(value)
	case message.Field_INTEGER:
		if ai >= len(field.ValueInteger) {
			return fieldType, unsafe.Pointer(nil), 0
		}
		return fieldType, unsafe.Pointer(&field.ValueInteger[ai]), 0
	case message.Field_DOUBLE:
		if ai >= len(field.ValueDouble) {
			return fieldType, unsafe.Pointer(nil), 0
		}
		return fieldType, unsafe.Pointer(&field.ValueDouble[ai]), 0
	case message.Field_BOOL:
		if ai >= len(field.ValueBool) {
			return fieldType, unsafe.Pointer(nil), 0
		}
		return fieldType, unsafe.Pointer(&field.ValueBool[ai]), 0
	}
	return 0, unsafe.Pointer(nil), 0
}

//export go_lua_read_message
func go_lua_read_message(ptr unsafe.Pointer, c *C.char, fi, ai int) (int, unsafe.Pointer,
	int) {
	fieldName := C.GoString(c)
	var lsb *LuaSandbox = (*LuaSandbox)(ptr)
	if lsb.msg != nil {
		switch fieldName {
		case "Type":
			value := lsb.msg.GetType()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "Logger":
			value := lsb.msg.GetLogger()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "Payload":
			value := lsb.msg.GetPayload()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "EnvVersion":
			value := lsb.msg.GetEnvVersion()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "Hostname":
			value := lsb.msg.GetHostname()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "Uuid":
			value := lsb.msg.GetUuidString()
			cs := C.CString(value) // freed by the caller
			return int(message.Field_STRING), unsafe.Pointer(cs),
				len(value)
		case "Timestamp":
			return int(message.Field_INTEGER),
				unsafe.Pointer(lsb.msg.Timestamp), 0
		case "Severity":
			return int(message.Field_INTEGER),
				unsafe.Pointer(lsb.msg.Severity), 0
		case "Pid":
			return int(message.Field_INTEGER),
				unsafe.Pointer(lsb.msg.Severity), 0
		default:
			l := len(fieldName)
			if l > 0 && fieldName[l-1] == ']' {
				if strings.HasPrefix(fieldName, "Fields[") {
					t, p, l := lookup_field(lsb.msg, fieldName[7:l-1], fi, ai)
					return t, p, l
				} else if strings.HasPrefix(fieldName, "Captures[") {
					value, ok := lsb.captures[fieldName[9:l-1]]
					if ok {
						cs := C.CString(value) // freed by the caller
						return int(message.Field_STRING), unsafe.Pointer(cs),
							len(value)
					}
				}
			}
		}
	}
	return 0, unsafe.Pointer(nil), 0
}

//export go_lua_inject_message
func go_lua_inject_message(ptr unsafe.Pointer, c *C.char) {
	var lsb *LuaSandbox = (*LuaSandbox)(ptr)
	lsb.injectMessage(C.GoString(c))
}

type LuaSandbox struct {
	lsb           *C.lua_sandbox
	msg           *message.Message
	captures      map[string]string
	output        func(s string)
	injectMessage func(s string)
}

func CreateLuaSandbox(conf *sandbox.SandboxConfig) (sandbox.Sandbox,
	error) {
	lsb := new(LuaSandbox)
	cs := C.CString(conf.ScriptFilename)
	defer C.free(unsafe.Pointer(cs))
	lsb.lsb = C.lua_sandbox_create(unsafe.Pointer(lsb),
		cs,
		C.uint(conf.MemoryLimit),
		C.uint(conf.InstructionLimit),
		C.uint(conf.OutputLimit))
	if lsb.lsb == nil {
		return nil, fmt.Errorf("Sandbox creation failed")
	}
	lsb.output = func(s string) { log.Println(s) }
	lsb.injectMessage = func(s string) { log.Println(s) }
	return lsb, nil
}

func (this *LuaSandbox) Init(dataFile string) error {
	cs := C.CString(dataFile)
	defer C.free(unsafe.Pointer(cs))
	r := int(C.lua_sandbox_init(this.lsb, cs))
	if r != 0 {
		return fmt.Errorf("Init() %s", this.LastError())
	}
	return nil
}

func (this *LuaSandbox) Destroy(dataFile string) error {
	cs := C.CString(dataFile)
	defer C.free(unsafe.Pointer(cs))
	c := C.lua_sandbox_destroy(this.lsb, cs)
	if c != nil {
		err := C.GoString(c)
		C.free(unsafe.Pointer(c))
		return fmt.Errorf("Destroy() %s", err)
	}
	return nil
}

func (this *LuaSandbox) Status() int {
	return int(C.lua_sandbox_status(this.lsb))
}

func (this *LuaSandbox) LastError() string {
	return C.GoString(C.lua_sandbox_last_error(this.lsb))
}

func (this *LuaSandbox) Usage(utype, ustat int) uint {
	return uint(C.lua_sandbox_usage(this.lsb, C.sandbox_usage_type(utype),
		C.sandbox_usage_stat(ustat)))
}

func (this *LuaSandbox) ProcessMessage(msg *message.Message,
	captures map[string]string) int {
	this.msg = msg
	this.captures = captures
	r := int(C.lua_sandbox_process_message(this.lsb))
	this.captures = nil
	this.msg = nil
	return r
}

func (this *LuaSandbox) TimerEvent(ns int64) int {
	return int(C.lua_sandbox_timer_event(this.lsb, C.longlong(ns)))
}

func (this *LuaSandbox) InjectMessage(f func(s string)) {
	this.injectMessage = f
}

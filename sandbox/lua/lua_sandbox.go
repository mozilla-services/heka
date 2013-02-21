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
#cgo CFLAGS: -std=gnu99
#cgo LDFLAGS: -L ./ -llua_sandbox -lm
#include <stdlib.h>
#include "lua_sandbox.h"
*/
import "C"

import "fmt"
import "unsafe"

//export lua_sandbox_print
func lua_sandbox_print(ptr unsafe.Pointer, s string) {
	var lsb *LuaSandbox = (*LuaSandbox)(ptr)
	lsb.Print(s)
}

//export lua_sandbox_send_message
func lua_sandbox_send_message(ptr unsafe.Pointer, s string) {
	var lsb *LuaSandbox = (*LuaSandbox)(ptr)
	lsb.SendMessage(s)
}

type LuaSandbox struct {
	lsb *C.lua_sandbox
}

func CreateLuaSandbox(code string, maxMem, maxInst int) (*LuaSandbox, error) {
	lsb := new(LuaSandbox)
	cs := C.CString(code)
	defer C.free(unsafe.Pointer(cs))
	lsb.lsb = C.lua_sandbox_create(unsafe.Pointer(lsb),
		cs,
		C.uint(maxMem),
		C.uint(maxInst))
	if lsb.lsb == nil {
		return nil, fmt.Errorf("Sandbox creation failed")
	}
	return lsb, nil
}

func (this *LuaSandbox) Init() error {
	r := int(C.lua_sandbox_init(this.lsb))
	if r != 0 {
		return fmt.Errorf("Init() %s", this.LastError())
	}
	return nil
}

func (this *LuaSandbox) Destroy() {
	C.lua_sandbox_destroy(this.lsb)
}

func (this *LuaSandbox) Status() int {
	return int(C.lua_sandbox_status(this.lsb))
}

func (this *LuaSandbox) LastError() string {
	return C.GoString(C.lua_sandbox_last_error(this.lsb))
}

func (this *LuaSandbox) Memory(usage int) int {
	return int(C.lua_sandbox_memory(this.lsb, C.sandbox_usage(usage)))
}

func (this *LuaSandbox) Instructions(usage int) int {
	return int(C.lua_sandbox_instructions(this.lsb, C.sandbox_usage(usage)))
}

func (this *LuaSandbox) ProcessMessage(msg string) int {
	cs := C.CString(msg)
	defer C.free(unsafe.Pointer(cs))
	return int(C.lua_sandbox_process_message(this.lsb, cs))
}

func (this *LuaSandbox) TimerEvent() int {
	return int(C.lua_sandbox_timer_event(this.lsb))
}

func (this *LuaSandbox) Print(s string) {
	// @todo print somewhere else
	fmt.Println(s)
}

func (this *LuaSandbox) SendMessage(msg string) {
	// @todo unmarshal message and put it in a stream
}

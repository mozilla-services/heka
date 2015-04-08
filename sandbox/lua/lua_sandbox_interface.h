/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Heka Go interfaces for the Lua sandbox @file
#ifndef lua_sandbox_interface_
#define lua_sandbox_interface_

#include <luasandbox.h>
#include <luasandbox/lua.h>

// LMW_ERR_*: Lua Message Write errors
extern const int LMW_ERR_NO_SANDBOX_PACK;
extern const int LMW_ERR_WRONG_TYPE;
extern const int LMW_ERR_NEWFIELD_FAILED;
extern const int LMW_ERR_BAD_FIELD_INDEX;
extern const int LMW_ERR_BAD_ARRAY_INDEX;
extern const int LMW_ERR_INVALID_FIELD_NAME;

/**
* Passes a Heka message down to the sandbox for processing. The instruction
* count limits are active during this call.
*
* @param lsb Pointer to the sandbox
*
* @return int Zero on success, non-zero on failure.
*/
int process_message(lua_sandbox* lsb);

/**
* Called when the plugin timer expires (the garbage collector is run after
* its execution). The instruction count limits are active during this call.
*
* @param lsb Pointer to the sandbox.
*
* @return int Zero on success, non-zero on failure.
*
*/
int timer_event(lua_sandbox* lsb, long long ns);

/**
* Reads a configuration variable provided in the Heka toml and returns the
* value.
*
* @param lua Pointer to the Lua state.
*
* @return int Returns one value on the stack.
*/
int read_config(lua_State* lua);

/**
* Reads a data field from a Heka message and returns the value.
*
* @param lua Pointer to the Lua state.
*
* @return int Returns one value on the stack.
*/
int read_message(lua_State* lua);

/**
 * Iterates through the message fields returning the type, name, value,
 * representation, and count for each field.
 *
 * @param lua Pointer to the Lua state.
 *
 * @return int Returns five values on the stack.
 */
int read_next_field(lua_State* lua);

/**
* Inject a message into Heka using the output buffer's contents as the message
* payload.
*
* @param lua Pointer to the Lua state.
*
* @return int Returns zero values on the stack.
*/
int inject_message(lua_State* lua);

/**
 * Initializes the sandbox and sets up the above callbacks.
 *
 * @param lsb Pointer to the sandbox.
 * @param data_file File used for the data restoration (empty or NULL for no
 *                  restoration)
 *
 * @return int 0 on success
 */
int sandbox_init(lua_sandbox* lsb, const char* data_file, const char* plugin_type);

/**
 * Sends a shutdown message to the sandbox.
 *
 * @param lsb Pointer to the sandbox.
 *
 */
void sandbox_stop(lua_sandbox* lsb);

#endif


/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Lua lua_sandbox for Heka plugins @file
#ifndef lua_sandbox_h_
#define lua_sandbox_h_

#include <lua.h>
#include "../sandbox.h"

typedef struct lua_sandbox lua_sandbox;

/**
 * Allocates and initializes the structure around the Lua sandbox.
 * 
 * @param go Pointer to associate the Go struct to this sandbox.
 * @param lua_file Filename of the Lua script to run in this sandbox.
 * @param memory_limit Sets the sandbox memory limit (bytes).
 * @param instruction_limit Sets the sandbox Lua instruction limit (count). 
 * This limit is per call to process_message or timer_event 
 * @param output_limit Sets the single message payload limit (bytes). This 
 * limit applies to the in memory output buffer.  The buffer is reset back 
 * to zero when inject_message is called. 
 * 
 * @return lua_sandbox Sandbox pointer or NULL on failure.
 */
lua_sandbox* lua_sandbox_create(void* go,
                                const char* lua_file,
                                unsigned memory_limit,
                                unsigned instruction_limit,
                                unsigned output_limit);

/**
 * Frees the memory associated with the sandbox.
 * 
 * @param lsb        Sandbox pointer to discard.
 * @param state_file Filename where the sandbox global data is saved. Use a 
 * NULL or empty string for no preservation.
 * 
 * @return NULL on success, pointer to an error message on failure that MUST BE
 * FREED by the caller.
 */
char* lua_sandbox_destroy(lua_sandbox* lsb,
                          const char* state_file);

/** 
 * Initializes the Lua sandbox and loads/runs the Lua script that was specified 
 * in lua_create_sandbox.
 * 
 * @param lsb Pointer to the sandbox.
 * @param state_file Filename where the global data is read. Use a NULL or empty
 *                   string no data restoration.
 * 
 * @return int Zero on success, non-zero on failure.
 */
int lua_sandbox_init(lua_sandbox* lsb, const char* state_file);

/** 
 * Retrieve the sandbox usage statistics.
 * 
 * @param lsb Pointer to the sandbox.
 * @param sandbox_usage_type Type of statistic to retrieve i.e. memory.
 * @param sandbox_usage_stat Type of statistic to retrieve i.e. current.
 * 
 * @return unsigned Count or number of bytes depending on the statistic.
 */
unsigned lua_sandbox_usage(lua_sandbox* lsb,
                           sandbox_usage_type utype,
                           sandbox_usage_stat ustat);
/**
 * Retrieve the current sandbox status.
 * 
 * @param lsb    Pointer to the sandbox.
 * 
 * @return sandbox_status code
 */
sandbox_status lua_sandbox_status(lua_sandbox* lsb);

/** 
 * Return the last error in human readable form.
 * 
 * @param lsb Pointer to the sandbox.
 * 
 * @return const char* error message
 */
const char* lua_sandbox_last_error(lua_sandbox* lsb);

/**
 * Passes a Heka message down to the sandbox for processing. The instruction 
 * count limits are active during this call. 
 * 
 * @param lsb Pointer to the sandbox
 * 
 * @return int Zero on success, non-zero on failure.
 */
int lua_sandbox_process_message(lua_sandbox* lsb);

/**
 * Called when the plugin timer expires (the garbage collector is run after 
 * its execution). The instruction count limits are active during this call. 
 * 
 * @param lsb Pointer to the sandbox.
 *  
 * @return int Zero on success, non-zero on failure.
 * 
 */
int lua_sandbox_timer_event(lua_sandbox* lsb, long long ns);

#endif

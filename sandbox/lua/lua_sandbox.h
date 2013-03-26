/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Lua lua_sandbox for Heka plugins @file
#ifndef lua_sandbox_h_
#define lua_sandbox_h_

#include "../sandbox.h"

typedef struct lua_sandbox lua_sandbox;

/**
 * Allocates and initializes the structure around the Lua sandbox
 * 
 * @param go pointer to associate the Go struct to this sandbox
 * @param lua_file filename of the Lua script to run in this sandbox
 * @param mem_lim  sets the sandbox memory limit (bytes)
 * @param ins_lim sets the sandbox Lua instruction limit (count)
 * @param out_lim sets the single message payload limit (bytes)
 * 
 * @return lua_sandbox
 */
SANDBOX_EXPORT lua_sandbox* lua_sandbox_create(void* go,
                                               const char* lua_file,
                                               unsigned mem_lim,
                                               unsigned ins_lim,
                                               unsigned out_lim);

/**
 * Frees the memory associated with the sandbox
 * 
 * @param lsb        sandbox pointer to discard
 * @param state_file filename to save the sandbox state to (empty or NULL for no
 *                   preservation)
 * 
 * @return NULL on success, pointer to an error message on failure (MUST BE 
 * FREED by the caller) 
 */
SANDBOX_EXPORT char* lua_sandbox_destroy(lua_sandbox* lsb,
                                         const char* state_file);

/** 
 * Initialize the Lua sandbox and loads/runs the Lua script that was specified 
 * in lua_create_sandbox 
 * 
 * @param lsb pointer to the sandbox
 * @param state_file filename to read the sandbox state from (empty or NULL for 
 *                   no restoration)
 * 
 * @return int 0 on success
 */
SANDBOX_EXPORT int lua_sandbox_init(lua_sandbox* lsb, const char* state_file);

/** 
 * Retrieve the sandbox memory statistics
 * 
 * @param lsb pointer to the sandbox
 * @param sandbox_usage_type
 * @param sandbox_usage_stat
 * 
 * @return unsigned number of bytes of memory
 */
SANDBOX_EXPORT unsigned lua_sandbox_usage(lua_sandbox* lsb,
                                          sandbox_usage_type utype, 
                                          sandbox_usage_stat ustat);
/**
 * Sandbox status
 * 
 * @param lsb    pointer to the sandbox
 * 
 * @return status code
 */
SANDBOX_EXPORT sandbox_status lua_sandbox_status(lua_sandbox* lsb);

/** 
 * Human readable error message
 * 
 * @param lsb pointer to the sandbox
 * 
 * @return const char* error message
 */
SANDBOX_EXPORT const char* lua_sandbox_last_error(lua_sandbox* lsb);

/**
 * Passes a Heka message down to the sandbox for processing
 * 
 * @param lsb pointer to the sandbox
 * 
 * @return 0 on success
 */
SANDBOX_EXPORT int lua_sandbox_process_message(lua_sandbox* lsb);

/**
 * Called when the plugin timer expires (the garbage collector is run after 
 * its execution) 
 * 
 * @param lsb pointer to the sandbox 
 *  
 * @return 0 on success
 * 
 */
SANDBOX_EXPORT int lua_sandbox_timer_event(lua_sandbox* lsb, long long ns);

#endif

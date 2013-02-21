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
 * @param mem_limit  sets the sandbox memory limit
 * @param inst_limit sets the sandbox Lua instruction limit
 * 
 * @return lua_sandbox
 */
SANDBOX_EXPORT lua_sandbox* lua_sandbox_create(void* go,
                                               const char* lua_file,
                                               unsigned mem_limit,
                                               unsigned inst_limit);

/** 
 * Frees the memory associated with the sandbox
 * 
 * @param lsb sandbox pointer to discard
 */
SANDBOX_EXPORT void lua_sandbox_destroy(lua_sandbox* lsb);

/** 
 * Initialize the Lua sandbox and loads/runs the Lua script that was specified 
 * in lua_create_sandbox 
 * 
 * @param lsb pointer to the sandbox
 * 
 * @return int 0 on success
 */
SANDBOX_EXPORT int lua_sandbox_init(lua_sandbox* lsb);

/** 
 * Retrieve the sandbox memory statistics
 * 
 * @param lsb pointer to the sandbox
 * @param sandbox_usage requested statistic
 * 
 * @return unsigned number of bytes of memory
 */
SANDBOX_EXPORT unsigned lua_sandbox_memory(lua_sandbox* lsb,
                                           sandbox_usage stat);

/** 
 * Retrieve the sandbox instruction statistics
 * 
 * @param lsb pointer to the sandbox
 * @param sandbox_usage requested statistic
 * 
 * @return unsigned number of Lua instructions
 */
SANDBOX_EXPORT unsigned lua_sandbox_instructions(lua_sandbox* lsb,
                                                 sandbox_usage stat);

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
 * @param msg protocol buffer encoded message
 * 
 * @return 0 on success
 */
SANDBOX_EXPORT int lua_sandbox_process_message(lua_sandbox* lsb,
                                               const char* msg);

/**
 * Called when the plugin timer expires (the garbage collector is run after 
 * its execution) 
 * 
 * @param lsb pointer to the sandbox 
 *  
 * @return 0 on success
 * 
 */
SANDBOX_EXPORT int lua_sandbox_timer_event(lua_sandbox* lsb);

#endif

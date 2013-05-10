/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Lua circular buffer - time series data store for Heka plugins @file
#ifndef lua_circular_buffer_h_
#define lua_circular_buffer_h_

#include <lua.h>
#include "lua_sandbox_private.h"

extern const char* heka_circular_buffer;
extern const char* heka_circular_buffer_table;
typedef struct circular_buffer circular_buffer;

/**
 * Output the circular buffer user data
 * 
 * @param cb Circular buffer userdata object.
 * @param output Output stream where the data is written.
 * @return Zero on success
 * 
 */
int output_circular_buffer(circular_buffer* cb, output_data* output);

/**
 * Serialize the circular buffer user data
 *  
 * @param key Lua variable name.
 * @param cb  Circular buffer userdata object.
 * @param output Output stream where the data is written.
 * @return Zero on success
 * 
 */
int serialize_circular_buffer(const char* key, circular_buffer* cb, 
                              output_data* output);

/**
 * Circular buffer library loader
 * 
 * @param lua Lua state
 * 
 */
void luaopen_circular_buffer(lua_State *lua);


#endif

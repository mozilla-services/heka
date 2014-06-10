/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
#include <stdlib.h>
#include <stdio.h>
#include <ctype.h>
#include <string.h>
#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>
#include <time.h>
#include <lua_sandbox.h>
#include "_cgo_export.h"

////////////////////////////////////////////////////////////////////////////////
/// Calls to Lua
////////////////////////////////////////////////////////////////////////////////
int process_message(lua_sandbox* lsb)
{
    static const char* func_name = "process_message";
    lua_State* lua = lsb_get_lua(lsb);
    if (!lua) return 1;

    if (lsb_pcall_setup(lsb, func_name)) {
        char err[LSB_ERROR_SIZE];
        snprintf(err, LSB_ERROR_SIZE, "%s() function was not found", func_name);
        lsb_terminate(lsb, err);
        return 1;
    }

    if (lua_pcall(lua, 0, 1, 0) != 0) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              lua_tostring(lua, -1));
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }

    if (!lua_isnumber(lua, 1)) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE,
                              "%s() must return a single numeric value", func_name);
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }

    int status = (int)lua_tointeger(lua, 1);
    lua_pop(lua, 1);

    lsb_pcall_teardown(lsb);

    return status;
}

////////////////////////////////////////////////////////////////////////////////
int timer_event(lua_sandbox* lsb, long long ns)
{
    static const char* func_name = "timer_event";
    lua_State* lua = lsb_get_lua(lsb);
    if (!lua) return 1;

    if (lsb_pcall_setup(lsb, func_name)) {
        char err[LSB_ERROR_SIZE];
        snprintf(err, LSB_ERROR_SIZE, "%s() function was not found", func_name);
        lsb_terminate(lsb, err);
        return 1;
    }

    lua_pushnumber(lua, ns);
    if (lua_pcall(lua, 1, 0, 0) != 0) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              lua_tostring(lua, -1));
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }
    lsb_pcall_teardown(lsb);
    lua_gc(lua, LUA_GCCOLLECT, 0);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
/// Calls from Lua
////////////////////////////////////////////////////////////////////////////////
int read_config(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "read_config() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    if (lua_gettop(lua) != 1) {
        luaL_error(lua, "read_config() must have a single argument");
    }
    const char* name = luaL_checkstring(lua, 1);

    struct go_lua_read_config_return gr;
    // Cast away constness of the Lua string, the value is not modified
    // and it will save a copy.
    gr = go_lua_read_config(lsb_get_parent(lsb), (char*)name);
    if (gr.r1 == NULL) {
        lua_pushnil(lua);
    } else {
        switch (gr.r0) {
        case 0:
            lua_pushlstring(lua, gr.r1, gr.r2);
            free(gr.r1);
            break;
        case 3:
            lua_pushnumber(lua, *((GoFloat64*)gr.r1));
            break;
        case 4:
            lua_pushboolean(lua, *((GoInt8*)gr.r1));
            break;
        default:
            lua_pushnil(lua);
            break;
        }
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int read_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "read_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (n < 1 || n > 3) {
        luaL_error(lua, "read_message() incorrect number of arguments");
    }
    const char* field = luaL_checkstring(lua, 1);
    int fi = luaL_optinteger(lua, 2, 0);
    luaL_argcheck(lua, fi >= 0, 2, "field index must be >= 0");
    int ai = luaL_optinteger(lua, 3, 0);
    luaL_argcheck(lua, ai >= 0, 3, "array index must be >= 0");

    struct go_lua_read_message_return gr;
    // Cast away constness of the Lua string, the value is not modified
    // and it will save a copy.
    gr = go_lua_read_message(lsb_get_parent(lsb), (char*)field, fi, ai);
    if (gr.r1 == NULL) {
        lua_pushnil(lua);
    } else {
        switch (gr.r0) {
        case 0:
            lua_pushlstring(lua, gr.r1, gr.r2);
            free(gr.r1);
            break;
        case 1:
            lua_pushlstring(lua, gr.r1, gr.r2);
            break;
        case 2:
            if (strncmp("Pid", field, 3) == 0
                || strncmp("Severity", field, 8) == 0) {
                lua_pushinteger(lua, *((GoInt32*)gr.r1));
            } else {
                lua_pushnumber(lua, *((GoInt64*)gr.r1));
            }
            break;
        case 3:
            lua_pushnumber(lua, *((GoFloat64*)gr.r1));
            break;
        case 4:
            lua_pushboolean(lua, *((GoInt8*)gr.r1));
            break;
        default:
            lua_pushnil(lua);
            break;
        }
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int write_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "write_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (n < 2 || n > 5) {
        luaL_error(lua, "write_message() incorrect number of arguments");
    }
    const char* field = luaL_checkstring(lua, 1);
    int type = lua_type(lua, 2);
    const char* rep = luaL_optstring(lua, 3, "");
    int fi = luaL_optinteger(lua, 4, 0);
    luaL_argcheck(lua, fi >= 0, 4, "field index must be >= 0");
    int ai = luaL_optinteger(lua, 5, 0);
    luaL_argcheck(lua, ai >= 0, 5, "array index must be >= 0");

    int result;

    // In all of the `go_lua_write_message` calls below we cast away constness
    // of the Lua strings since the values are not modified and doing so will
    // save copies.
    switch (type) {
    case LUA_TBOOLEAN: {
        int value = lua_toboolean(lua, 2);
        result = go_lua_write_message_bool(lsb_get_parent(lsb), (char*)field,
            value, (char*)rep, fi, ai);
        break;
    }
    case LUA_TNUMBER: {
        lua_Number value = lua_tonumber(lua, 2);
            result = go_lua_write_message_double(lsb_get_parent(lsb), (char*)field,
                value, (char*)rep, fi, ai);
        break;
    }
    case LUA_TSTRING: {
        size_t len;
        const char* value = lua_tostring(lua, 2);
        result = go_lua_write_message_string(lsb_get_parent(lsb), (char*)field,
            (char*)value, (char*)rep, fi, ai);
        break;
    }
    default:
        luaL_error(lua, "write_message() only accepts numeric, string, or boolean field values");
    }

    if (result != 0) {
        luaL_error(lua, "write_message() failed");
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int read_next_field(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "read_next_field() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    if (lua_gettop(lua) != 0) {
        luaL_error(lua, "read_next_field() takes no arguments");
    }

    struct go_lua_read_next_field_return gr;
    gr = go_lua_read_next_field(lsb_get_parent(lsb));
    if (gr.r3 == NULL) {
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
    } else {
        lua_pushinteger(lua, gr.r0); // type
        lua_pushlstring(lua, gr.r1, gr.r2); // name
        free(gr.r1);
        switch (gr.r0) { // value
        case 0:
            lua_pushlstring(lua, gr.r3, gr.r4);
            free(gr.r3);
            break;
        case 1:
            lua_pushlstring(lua, gr.r3, gr.r4);
            break;
        case 2:
            lua_pushnumber(lua, *((GoInt64*)gr.r3));
            break;
        case 3:
            lua_pushnumber(lua, *((GoFloat64*)gr.r3));
            break;
        case 4:
            lua_pushboolean(lua, *((GoInt8*)gr.r3));
            break;
        default:
            lua_pushnil(lua);
            break;
        }
        lua_pushlstring(lua, gr.r5, gr.r6); // representation
        free(gr.r5);
        lua_pushinteger(lua, gr.r7); // count
    }
    return 5;
}

////////////////////////////////////////////////////////////////////////////////
int inject_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "inject_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    if (lua_gettop(lua) != 1 || lua_type(lua, 1) != LUA_TTABLE) {
        luaL_error(lua, "inject_message() takes a single table argument");
    }

    if (lsb_output_protobuf(lsb, 1, 0) != 0) {
      luaL_error(lua, "inject_message() could not encode protobuf - %s",
                 lsb_get_error(lsb));
    }
    size_t len;
    const char* output = lsb_get_output(lsb, &len);

    if (len != 0) {
        int result = go_lua_inject_message(lsb_get_parent(lsb),
                                           (char*)output,
                                           (int)len,
                                           "",
                                           "");
        if (result != 0) {
            luaL_error(lua, "inject_message() exceeded MaxMsgLoops");
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int inject_payload(lua_State* lua)
{
    static const char* default_type = "txt";

    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "inject_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    const char* type = default_type;
    const char* name = "";

    int n = lua_gettop(lua);
    if (n > 0) {
        size_t len = 0;
        type = luaL_checklstring(lua, 1, &len);
        if (len == 0) type = default_type;
    }
    if (n > 1) {
        name = luaL_checkstring(lua, 2);
    }
    if (n > 2) {
        lsb_output(lsb, 3, n, 1);
    }
    size_t len;
    const char* output = lsb_get_output(lsb, &len);

    if (len != 0) {
        int result = go_lua_inject_message(lsb_get_parent(lsb),
                                           (char*)output,
                                           (int)len,
                                           (char*)type,
                                           (char*)name);
        if (result != 0) {
            luaL_error(lua, "inject_payload() exceeded MaxMsgLoops");
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_init(lua_sandbox* lsb, const char* data_file, const char* plugin_type)
{
    static const char *output = "output";
    if (!lsb) return 1;

    lsb_add_function(lsb, &read_config, "read_config");
    lsb_add_function(lsb, &read_message, "read_message");
    lsb_add_function(lsb, &read_next_field, "read_next_field");
    lsb_add_function(lsb, &inject_payload, "inject_payload");
    lsb_add_function(lsb, &inject_message, "inject_message");

    if (strcmp(plugin_type, "decoder") == 0 ||
        strcmp(plugin_type, "encoder") == 0) {
        lsb_add_function(lsb, &write_message, "write_message");
    }

    int result = lsb_init(lsb, data_file);
    if (result) return result;

    // rename output to add_to_payload
    lua_State* lua = lsb_get_lua(lsb);
    lua_getglobal(lua, output);
    lua_setglobal(lua, "add_to_payload");
    lua_pushnil(lua);
    lua_setglobal(lua, output);

    return 0;
}

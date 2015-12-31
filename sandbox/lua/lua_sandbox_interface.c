/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
#include <ctype.h>
#include <luasandbox.h>
#include <luasandbox/lauxlib.h>
#include <luasandbox/lua.h>
#include <luasandbox/lualib.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
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

    if (lua_pcall(lua, 0, 2, 0) != 0) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              lua_tostring(lua, -1));
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        // Don't terminate if we're aborting so we preserve data on exit.
        const char* tail = err + len -7; // Last 7 chars of err msg.
        if (strcmp(tail, "aborted") != 0) {
          lsb_terminate(lsb, err);
        }
        return 1;
    }

    if (lua_type(lua, 1) != LUA_TNUMBER) {
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE,
                              "%s() must return a numeric status code",
                              func_name);
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        lsb_terminate(lsb, err);
        return 1;
    }

    int status = (int)lua_tointeger(lua, 1);
    switch (lua_type(lua, 2)) {
    case LUA_TNIL:
        lsb_set_error(lsb, NULL);
        break;
    case LUA_TSTRING:
        lsb_set_error(lsb, lua_tostring(lua, 2));
        break;
    default:
        {
            char err[LSB_ERROR_SIZE];
            int len = snprintf(err, LSB_ERROR_SIZE,
                    "%s() must return a nil or string error message",
                    func_name);
            if (len >= LSB_ERROR_SIZE || len < 0) {
                err[LSB_ERROR_SIZE - 1] = 0;
            }
            lsb_terminate(lsb, err);
            return 1;
        }
        break;
    }
    lua_pop(lua, 2);

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
        size_t errmsg_len = 0;
        const char* errmsg = lua_tolstring(lua, -1, &errmsg_len);
        char err[LSB_ERROR_SIZE];
        size_t len = snprintf(err, LSB_ERROR_SIZE, "%s() %s", func_name,
                              errmsg);
        if (len >= LSB_ERROR_SIZE) {
          err[LSB_ERROR_SIZE - 1] = 0;
        }
        // Don't terminate if we're aborting so we preserve data on exit.
        if (errmsg_len < 7) {
            lsb_terminate(lsb, err);
        } else {
            const char* tail = errmsg + strlen(errmsg) - 7; // Last 7 chars of err msg.
            if (strcmp(tail, "aborted") != 0) {
                lsb_terminate(lsb, err);
            }
        }
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
    int has_ai = !lua_isnoneornil(lua, 5); // needed for deletion

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
    case LUA_TNIL: {
        result = go_lua_delete_message_field(lsb_get_parent(lsb), (char*)field, fi, ai, has_ai);
        break;
    }
    default:
        luaL_error(lua, "write_message() only accepts numeric, string, or boolean field values, or nil to delete");
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
    if (!gr.r1) { // a null name is the end of the field array
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
        lua_pushnil(lua);
        return 5;
    }

    lua_pushinteger(lua, gr.r0); // type
    lua_pushlstring(lua, gr.r1, gr.r2); // name
    free(gr.r1);
    switch (gr.r0) { // value
    case 0:
        if (gr.r3) {
            lua_pushlstring(lua, gr.r3, gr.r4);
            free(gr.r3);
        } else {
            lua_pushnil(lua);
        }
        break;
    case 1:
        if (gr.r3) {
            lua_pushlstring(lua, gr.r3, gr.r4);
        } else {
            lua_pushnil(lua);
        }
        break;
    case 2:
        if (gr.r3) {
            lua_pushnumber(lua, *((GoInt64*)gr.r3));
        } else {
            lua_pushnil(lua);
        }
        break;
    case 3:
        if (gr.r3) {
            lua_pushnumber(lua, *((GoFloat64*)gr.r3));
        } else {
            lua_pushnil(lua);
        }
        break;
    case 4:
        if (gr.r3) {
            lua_pushboolean(lua, *((GoInt8*)gr.r3));
        } else {
            lua_pushnil(lua);
        }
        break;
    default:
        lua_pushnil(lua);
        break;
    }
    if (gr.r5) {
        lua_pushlstring(lua, gr.r5, gr.r6); // representation
        free(gr.r5);
    }
    lua_pushinteger(lua, gr.r7); // count

    return 5;
}

////////////////////////////////////////////////////////////////////////////////
static inline void inject_error(lua_State* lua, const char* fn, int result)
{
    switch (result) {
    case 0:
        break;
    case 1:
        luaL_error(lua, "%s protobuf unmarshal failed", fn);
        break;
    case 2:
        luaL_error(lua, "%s exceeded InjectMessage count", fn);
        break;
    case 3:
        luaL_error(lua, "%s exceeded MaxMsgLoops", fn);
        break;
    case 4:
        luaL_error(lua, "%s creates a circular reference (matches this plugin's message_matcher)", fn);
        break;
    case 5:
        luaL_error(lua, "%s aborted", fn);
    default:
        luaL_error(lua, "%s unknown error", fn);
        break;
    }
}


////////////////////////////////////////////////////////////////////////////////
int inject_message(lua_State* lua)
{
    static const char* fn = "inject_message()";

    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "%s invalid lightuserdata", fn);
    }

    int t = lua_type(lua, 1);
    if (lua_gettop(lua) != 1 || ( t != LUA_TSTRING && t != LUA_TTABLE)) {
        luaL_error(lua, "%s takes a single string or table argument", fn);
        return 1;
    }

    lua_sandbox* lsb = (lua_sandbox*)luserdata;
    size_t len = 0;
    const char* output = NULL;
    if (t == LUA_TSTRING) {
        output = lua_tolstring(lua, 1, &len);
    } else {
        if (lsb_output_protobuf(lsb, 1, 0) != 0) {
            const char *err = lsb_get_error(lsb);
            if (err[0] != 0) {
                luaL_error(lua, "%s could not encode protobuf - %s", fn, err);
            } else {
                luaL_error(lua, "%s output_limit exceeded", fn);
            }
        }
        output = lsb_get_output(lsb, &len);
    }

    if (output && len > 0) {
        int result = go_lua_inject_message(lsb_get_parent(lsb),
                                           (char*)output,
                                           (int)len,
                                           "",
                                           "");
        inject_error(lua, fn, result);
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int inject_payload(lua_State* lua)
{
    static const char* default_type = "txt";
    static const char* fn = "inject_payload()";

    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "%s invalid lightuserdata", fn);
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
        inject_error(lua, fn, result);
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_init(lua_sandbox* lsb, const char* data_file, const char* plugin_type)
{
    static const char *output = "output";
    if (!lsb || !plugin_type) return 1;

    int add_to_payload = 0;

    lsb_add_function(lsb, &read_config, "read_config");
    lsb_add_function(lsb, &lsb_decode_protobuf, "decode_message");

    if (strcmp(plugin_type, "input") == 0) {
        lsb_add_function(lsb, &inject_message, "inject_message");
    }

    if (strcmp(plugin_type, "output") == 0) {
        lsb_add_function(lsb, &read_message, "read_message");
        lsb_add_function(lsb, &read_next_field, "read_next_field");
    }

    if (strlen(plugin_type) == 0 // default an empty plugin type to filter
        || strcmp(plugin_type, "filter") == 0
        || strcmp(plugin_type, "decoder") == 0
        || strcmp(plugin_type, "encoder") == 0) {
        lsb_add_function(lsb, &read_message, "read_message");
        lsb_add_function(lsb, &read_next_field, "read_next_field");
        lsb_add_function(lsb, &inject_payload, "inject_payload");
        lsb_add_function(lsb, &inject_message, "inject_message");

        if (strcmp(plugin_type, "decoder") == 0 ||
            strcmp(plugin_type, "encoder") == 0) {
            lsb_add_function(lsb, &write_message, "write_message");
        }
        add_to_payload = 1;
    }

    int result = lsb_init(lsb, data_file);
    if (result) return result;

    lua_State* lua = lsb_get_lua(lsb);
    if (add_to_payload) {
        // rename output to add_to_payload
        lua_getglobal(lua, output);
        lua_setglobal(lua, "add_to_payload");
    }
    // remove the sandbox provided output function
    lua_pushnil(lua);
    lua_setglobal(lua, output);

    return 0;
}

////////////////////////////////////////////////////////////////////////////////
static void lstop (lua_State *L, lua_Debug *ar) {
  (void)ar;  /* unused arg. */
  lua_sethook(L, NULL, 0, 0);
  luaL_error(L, "shutting down");
}

////////////////////////////////////////////////////////////////////////////////
void sandbox_stop(lua_sandbox* lsb)
{
    lua_State* lua = lsb_get_lua(lsb);
    lua_sethook(lua, lstop, LUA_MASKCALL | LUA_MASKRET | LUA_MASKCOUNT, 1);
}

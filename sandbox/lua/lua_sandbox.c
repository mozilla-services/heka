/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
// gcc $(go env GOGCCFLAGS) -shared -std=gnu99 lua_sandbox.c -llua -o liblua_sandbox.so
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>
#include "lua_sandbox.h"
#include "_cgo_export.h"

////////////////////////////////////////////////////////////////////////////////
// private interface
////////////////////////////////////////////////////////////////////////////////
#define ERROR_SIZE 255
#define PRINT_SIZE 1024

struct lua_sandbox
{
    lua_State* m_lua;
    void*      m_go;

    unsigned m_memory_usage_current;
    unsigned m_memory_usage_maximum;
    unsigned m_memory_limit;

    unsigned m_instruction_usage_current;
    unsigned m_instruction_usage_maximum;
    unsigned m_instruction_limit;

    sandbox_status  m_status;
    char            m_error_message[ERROR_SIZE];

    char*    m_lua_file;
};

////////////////////////////////////////////////////////////////////////////////
void* sandbox_memory_manager(void* ud, void* ptr, size_t osize, size_t nsize)
{
    lua_sandbox* lsb = (lua_sandbox*)ud;

    void* nptr = NULL;
    if (nsize == 0) {
        free(ptr);
        lsb->m_memory_usage_current -= osize;
    } else {
        int new_state_memory = lsb->m_memory_usage_current + nsize - osize;
        if (new_state_memory <= lsb->m_memory_limit) {
            nptr = realloc(ptr, nsize);
            if (nptr != NULL) {
                lsb->m_memory_usage_current = new_state_memory;
                if (lsb->m_memory_usage_current > lsb->m_memory_usage_maximum) {
                    lsb->m_memory_usage_maximum = lsb->m_memory_usage_current;
                }
            }
        }
    }
    return nptr;
}

////////////////////////////////////////////////////////////////////////////////
void sandbox_instruction_manager(lua_State* lua, lua_Debug* ar)
{
    if (LUA_HOOKCOUNT == ar->event) {
        lua_pushstring(lua, "instruction_limit exceeded");
        lua_error(lua);
    }
}

////////////////////////////////////////////////////////////////////////////////
size_t sandbox_instruction_usage(lua_sandbox* lsb)
{
    return lua_gethookcount(lsb->m_lua) - lua_gethookcountremaining(lsb->m_lua);
}

////////////////////////////////////////////////////////////////////////////////
void sandbox_terminate(lua_sandbox* lsb)
{
    if (lsb->m_lua != NULL) {
        lua_close(lsb->m_lua);
        lsb->m_lua = NULL;
    }
    lsb->m_memory_usage_current = 0;
    lsb->m_status = STATUS_TERMINATED;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_read_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "read_message() invalid lightuserdata");
        lua_error(lua);
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (1 == n) {
        if (lua_isstring(lua, 1)) {
            size_t len = 0;
            const char* field = lua_tolstring(lua, 1, &len);
            struct go_read_message_return gr;
            // cast away constness, the value is not modified and will save a
            // copy
            gr = go_read_message(lsb->m_go, (char*)field);
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
                        lua_pushinteger(lua, *((GoInt*)gr.r1));
                    }
                    break;
                case 3:
                    lua_pushnumber(lua, *((GoFloat64*)gr.r1));
                    break;
                case 4:
                    lua_pushboolean(lua, *((GoInt8*)gr.r1));
                    break;
                }
            }
        }  else {
            lua_pushstring(lua, "read_message() argument must be a string");
            lua_error(lua);
        }
    } else {
        lua_pushstring(lua, "read_message() incorrect number of arguments");
        lua_error(lua);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_print(lua_State* lua)
{
    char buf[PRINT_SIZE]; ///@todo make this buffer dynamic, or print directly to a file
    buf[0] = 0;

    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "print() invalid lightuserdata");
        lua_error(lua);
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    size_t len = 0, total = 0;
    int n = lua_gettop(lua);
    if (n == 0) {
        lua_pushstring(lua, "print() must have at least one argument");
        lua_error(lua);
    }
    for (int i = 1; i <= n; ++i) {
        if (lua_isstring(lua, i)) { // this works for both strings and numbers
            const char* c = lua_tolstring(lua, i, &len);
            snprintf(buf, PRINT_SIZE - total, "%s", c);
            total += len;
        } else if (lua_isboolean(lua, i)) {
            if (lua_toboolean(lua, i)) {
                snprintf(buf, PRINT_SIZE - total, "true");
                total += 4;
            } else {
                snprintf(buf, PRINT_SIZE - total, "false");
                total += 5;
            }
        }
    }
    go_print(lsb->m_go, buf);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_send_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "send_message() invalid lightuserdata");
        lua_error(lua);
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (1 == n) {
        if (lua_isstring(lua, 1)) {
            size_t len = 0;
            const char* msg = lua_tolstring(lua, 1, &len);
            // cast away constness, the value is not modified and will save a
            // copy
            go_send_message(lsb->m_go, (char*)msg);
        }  else {
            lua_pushstring(lua, "send_message() argument must be a string");
            lua_error(lua);
        }
    } else {
        lua_pushstring(lua, "send_message() incorrect number of arguments");
        lua_error(lua);
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
void load_lua_libraries(lua_sandbox* lsb)
{
    // load base
    lua_pushcfunction(lsb->m_lua, luaopen_base);
    lua_pushstring(lsb->m_lua, "");
    lua_call(lsb->m_lua, 1, 0);
    // disable the base functions
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "collectgarbage");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "dofile");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "_G");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "getfenv");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "getmetatable");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "load");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "loadfile");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "loadstring");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "print");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawequal");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawget");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawset");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "setfenv");

    // load math
    lua_pushcfunction(lsb->m_lua, luaopen_math);
    lua_pushstring(lsb->m_lua, LUA_MATHLIBNAME);
    lua_call(lsb->m_lua, 1, 0);

    // load os
    lua_pushcfunction(lsb->m_lua, luaopen_os);
    lua_pushstring(lsb->m_lua, LUA_OSLIBNAME);
    lua_call(lsb->m_lua, 1, 0);
    // disable the os functions
    lua_getglobal(lsb->m_lua, "os");
    if (!lua_istable(lsb->m_lua, 1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "cannot disable the os functions");
        sandbox_terminate(lsb);
    }
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "execute");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "exit");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "remove");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "rename");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "setlocale");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, 1, "tmpname");
    lua_pop(lsb->m_lua, 1); // remove the os table

    // load string
    lua_pushcfunction(lsb->m_lua, luaopen_string);
    lua_pushstring(lsb->m_lua, LUA_STRLIBNAME);
    lua_call(lsb->m_lua, 1, 0);

    // load table
    lua_pushcfunction(lsb->m_lua, luaopen_table);
    lua_pushstring(lsb->m_lua, LUA_TABLIBNAME);
    lua_call(lsb->m_lua, 1, 0);
}


////////////////////////////////////////////////////////////////////////////////
// public interface
////////////////////////////////////////////////////////////////////////////////
lua_sandbox* lua_sandbox_create(void* go,
                                const char* lua_file,
                                unsigned mem_limit,
                                unsigned inst_limit)
{
    if (mem_limit > 1024*1024*8 || inst_limit > 1000000) {
        return NULL;
    }
    lua_sandbox* lsb = malloc(sizeof(lua_sandbox));
    if (lsb != NULL) {
        lsb->m_lua = NULL;
        lsb->m_go = go;
        lsb->m_memory_usage_current = 0;
        lsb->m_memory_usage_maximum = 0;
        lsb->m_memory_limit = mem_limit;
        lsb->m_instruction_usage_current = 0;
        lsb->m_instruction_usage_maximum = 0;
        lsb->m_instruction_limit = inst_limit;
        lsb->m_status = STATUS_UNKNOWN;
        lsb->m_error_message[0] = 0;
        size_t len = strlen(lua_file);
        lsb->m_lua_file = malloc(len + 1);
        lsb->m_lua_file[len] = 0;
        memcpy(lsb->m_lua_file, lua_file, len);
    }
    return lsb;
}

////////////////////////////////////////////////////////////////////////////////
void lua_sandbox_destroy(lua_sandbox* lsb)
{
    if (lsb != NULL) {
        sandbox_terminate(lsb);
        free(lsb->m_lua_file);
        free(lsb);
    }
}

////////////////////////////////////////////////////////////////////////////////
unsigned lua_sandbox_memory(lua_sandbox* lsb, sandbox_usage usage)
{
    if (lsb != NULL) {
        switch (usage) {
        case USAGE_LIMIT:
            return lsb->m_memory_limit;
        case USAGE_MAXIMUM:
            return lsb->m_memory_usage_maximum;
        default:
            return lsb->m_memory_usage_current;
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
unsigned lua_sandbox_instructions(lua_sandbox* lsb, sandbox_usage usage)
{
    if (lsb != NULL) {
        switch (usage) {
        case USAGE_LIMIT:
            return lsb->m_instruction_limit;
        case USAGE_MAXIMUM:
            return lsb->m_instruction_usage_maximum;
        default:
            return lsb->m_instruction_usage_current;
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
const char* lua_sandbox_last_error(lua_sandbox* lsb)
{
    if (lsb != NULL) {
        return lsb->m_error_message;
    }
    return "";
}

////////////////////////////////////////////////////////////////////////////////
sandbox_status lua_sandbox_status(lua_sandbox* lsb)
{
    if (lsb != NULL) {
        return lsb->m_status;
    }
    return STATUS_UNKNOWN;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_process_message(lua_sandbox* lsb)
{
    if (lsb == NULL || lsb->m_lua == NULL) {
        return 1;
    }

    lua_sethook(lsb->m_lua, sandbox_instruction_manager, LUA_MASKCOUNT,
                lsb->m_instruction_limit);
    lua_getglobal(lsb->m_lua, "process_message");
    if (!lua_isfunction(lsb->m_lua, -1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "process_message() function was not found");
        sandbox_terminate(lsb);
        return 1;
    }

    if (lua_pcall(lsb->m_lua, 0, 1, 0) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "process_message() -> %s", lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 1;
    }

    if (!lua_isnumber(lsb->m_lua, 1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "process_message() must return a single numeric value");
        sandbox_terminate(lsb);
        return 1;
    }

    int status = (int)lua_tointeger(lsb->m_lua, 1);
    lua_pop(lsb->m_lua, 1);
    lsb->m_instruction_usage_current = sandbox_instruction_usage(lsb);
    if (lsb->m_instruction_usage_current > lsb->m_instruction_usage_maximum) {
        lsb->m_instruction_usage_maximum = lsb->m_instruction_usage_current;
    }
    return status;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_timer_event(lua_sandbox* lsb)
{
    if (lsb == NULL || lsb->m_lua == NULL) {
        return 1;
    }

    lua_sethook(lsb->m_lua, sandbox_instruction_manager,
                LUA_MASKCOUNT,
                lsb->m_instruction_limit);
    lua_getglobal(lsb->m_lua, "timer_event");
    if (!lua_isfunction(lsb->m_lua, -1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "timer_event() function was not found");
        sandbox_terminate(lsb);
        return 1;
    }

    if (lua_pcall(lsb->m_lua, 0, 0, 0) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "timer_event() -> %s",
                 lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 1;
    }
    lsb->m_instruction_usage_current = sandbox_instruction_usage(lsb);
    if (lsb->m_instruction_usage_current > lsb->m_instruction_usage_maximum) {
        lsb->m_instruction_usage_maximum = lsb->m_instruction_usage_current;
    }
    lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_init(lua_sandbox* lsb)
{
    if (lsb == NULL || lsb->m_lua != NULL) {
        return 0;
    }
    if (lsb->m_lua_file == NULL) {
        snprintf(lsb->m_error_message, ERROR_SIZE, "no Lua script provided");
        sandbox_terminate(lsb);
        return 1;
    }
    lsb->m_lua = lua_newstate(sandbox_memory_manager, lsb);
    if (lsb->m_lua != NULL) {
        load_lua_libraries(lsb);

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_read_message, 1);
        lua_setglobal(lsb->m_lua, "read_message");

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_print, 1);
        lua_setglobal(lsb->m_lua, "print");

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_send_message, 1);
        lua_setglobal(lsb->m_lua, "send_message");
        lua_sethook(lsb->m_lua, sandbox_instruction_manager, LUA_MASKCOUNT,
                    lsb->m_instruction_limit);

        if (luaL_dofile(lsb->m_lua, lsb->m_lua_file) != 0) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "init() -> %s",
                     lua_tostring(lsb->m_lua, -1));
            sandbox_terminate(lsb);
            return 2;
        } else {
            lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
            lsb->m_instruction_usage_current = sandbox_instruction_usage(lsb);
            if (lsb->m_instruction_usage_current
                > lsb->m_instruction_usage_maximum) {
                lsb->m_instruction_usage_maximum =
                  lsb->m_instruction_usage_current;
            }
            lsb->m_status = STATUS_RUNNING;
        }
    } else {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "lua_newstate out of memory");
        sandbox_terminate(lsb);
        return 3;
    }
    return 0;
}

/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>
#include "lua_sandbox.h"
#include "lua_sandbox_private.h"
#include "lua_circular_buffer.h"

static const char* disable_base_functions[] = { "collectgarbage", "coroutine",
    "dofile", "getfenv", "getmetatable", "load", "loadfile", "loadstring",
    "module", "print", "rawequal", "require", "setfenv", NULL };

static const char* disable_os_functions[] = { "execute", "exit", "remove",
    "rename", "setlocale",  "tmpname", NULL };

static const char* disable_none[] = { NULL };

////////////////////////////////////////////////////////////////////////////////
lua_sandbox* lua_sandbox_create(void* go,
                                const char* lua_file,
                                unsigned memory_limit,
                                unsigned instruction_limit,
                                unsigned output_limit)
{
    if (memory_limit > MAX_MEMORY || instruction_limit > MAX_INSTRUCTION
        || output_limit > MAX_OUTPUT) {
        return NULL;
    }
    if (output_limit < OUTPUT_SIZE) {
        output_limit = OUTPUT_SIZE;
    }
    lua_sandbox* lsb = malloc(sizeof(lua_sandbox));
    if (lsb == NULL) return lsb;

    lsb->m_lua = NULL;
    lsb->m_go = go;
    memset(lsb->m_usage, 0, sizeof(lsb->m_usage));
    lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_LIMIT] = memory_limit;
    lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_LIMIT] = instruction_limit;
    lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_LIMIT] = output_limit;
    lsb->m_status = STATUS_UNKNOWN;
    lsb->m_error_message[0] = 0;
    lsb->m_output.m_pos = 0;
    lsb->m_output.m_size = OUTPUT_SIZE;
    lsb->m_output.m_data = malloc(lsb->m_output.m_size);
    size_t len = strlen(lua_file);
    lsb->m_lua_file = malloc(len + 1);
    if (lsb->m_output.m_data == NULL || lsb->m_lua_file == NULL) {
        free(lsb);
        return NULL;
    }
    strcpy(lsb->m_lua_file, lua_file);
    srand(time(NULL));
    return lsb;
}

////////////////////////////////////////////////////////////////////////////////
char* lua_sandbox_destroy(lua_sandbox* lsb, const char* data_file)
{
    char* err = NULL;
    if (lsb == NULL) return err;

    if (lsb->m_lua != NULL && data_file != NULL && strlen(data_file) > 0) {
        if (preserve_global_data(lsb, data_file) != 0) {
            size_t len = strlen(lsb->m_error_message);
            err = malloc(len + 1);
            if (err != NULL) {
                strcpy(err, lsb->m_error_message);
            }
        }
    }
    sandbox_terminate(lsb);
    free(lsb->m_output.m_data);
    free(lsb->m_lua_file);
    free(lsb);
    return err;
}

////////////////////////////////////////////////////////////////////////////////
unsigned lua_sandbox_usage(lua_sandbox* lsb, sandbox_usage_type utype,
                           sandbox_usage_stat ustat)
{
    if (utype >= MAX_USAGE_TYPE || ustat >= MAX_USAGE_STAT) {
        return 0;
    }
    return lsb->m_usage[utype][ustat];
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

    lua_sethook(lsb->m_lua, instruction_manager, LUA_MASKCOUNT,
                lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_LIMIT]);
    lua_getglobal(lsb->m_lua, "process_message");
    if (!lua_isfunction(lsb->m_lua, -1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "process_message() function was not found");
        sandbox_terminate(lsb);
        return 1;
    }

    if (lua_pcall(lsb->m_lua, 0, 1, 0) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "process_message() %s", lua_tostring(lsb->m_lua, -1));
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
    lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT] =
      instruction_usage(lsb);
    if (lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT]
        > lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM]) {
        lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM] =
          lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT];
    }
    return status;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_timer_event(lua_sandbox* lsb, long long ns)
{
    if (lsb == NULL || lsb->m_lua == NULL) {
        return 1;
    }

    lua_sethook(lsb->m_lua, instruction_manager, LUA_MASKCOUNT,
                lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_LIMIT]);
    lua_getglobal(lsb->m_lua, "timer_event");
    if (!lua_isfunction(lsb->m_lua, -1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "timer_event() function was not found");
        sandbox_terminate(lsb);
        return 1;
    }

    lua_pushnumber(lsb->m_lua, ns);
    if (lua_pcall(lsb->m_lua, 1, 0, 0) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "timer_event() %s",
                 lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 1;
    }
    lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT] =
      instruction_usage(lsb);
    if (lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT]
        > lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM]) {
        lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM] =
          lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT];
    }
    lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_init(lua_sandbox* lsb, const char* data_file)
{
    if (lsb == NULL || lsb->m_lua != NULL) {
        return 0;
    }
    if (lsb->m_lua_file == NULL) {
        snprintf(lsb->m_error_message, ERROR_SIZE, "no Lua script provided");
        sandbox_terminate(lsb);
        return 1;
    }
    lsb->m_lua = lua_newstate(memory_manager, lsb);
    if (lsb->m_lua == NULL) {
        snprintf(lsb->m_error_message, ERROR_SIZE, "out of memory");
        sandbox_terminate(lsb);
        return 2;
    }

    load_library(lsb->m_lua, "", luaopen_base, disable_base_functions);
    lua_pop(lsb->m_lua, 1);
    load_library(lsb->m_lua, LUA_MATHLIBNAME, luaopen_math, disable_none);
    lua_pop(lsb->m_lua, 1);
    load_library(lsb->m_lua, LUA_OSLIBNAME, luaopen_os, disable_os_functions);
    lua_pop(lsb->m_lua, 1);
    load_library(lsb->m_lua, LUA_STRLIBNAME, luaopen_string, disable_none);
    lua_pop(lsb->m_lua, 1);
    load_library(lsb->m_lua, LUA_TABLIBNAME, luaopen_table, disable_none);
    lua_pop(lsb->m_lua, 1);
    load_library(lsb->m_lua, heka_circular_buffer_table, 
                 luaopen_circular_buffer, disable_none);
    lua_pop(lsb->m_lua, 1);

    lua_pushcfunction(lsb->m_lua, &require_library);
    lua_setglobal(lsb->m_lua, "require");

    lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
    lua_pushcclosure(lsb->m_lua, &read_config, 1);
    lua_setglobal(lsb->m_lua, "read_config");

    lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
    lua_pushcclosure(lsb->m_lua, &read_message, 1);
    lua_setglobal(lsb->m_lua, "read_message");

    lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
    lua_pushcclosure(lsb->m_lua, &output, 1);
    lua_setglobal(lsb->m_lua, "output");

    lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
    lua_pushcclosure(lsb->m_lua, &inject_message, 1);
    lua_setglobal(lsb->m_lua, "inject_message");
    lua_sethook(lsb->m_lua, instruction_manager, LUA_MASKCOUNT,
                lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_LIMIT]);

    if (luaL_dofile(lsb->m_lua, lsb->m_lua_file) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE, "%s",
                 lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 3;
    } else {
        lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
        lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT] =
          instruction_usage(lsb);
        if (lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT]
            > lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM]) {
            lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_MAXIMUM] =
              lsb->m_usage[USAGE_TYPE_INSTRUCTION][USAGE_STAT_CURRENT];
        }
        lsb->m_status = STATUS_RUNNING;
        if (data_file != NULL && strlen(data_file) > 0) {
            return restore_global_data(lsb, data_file);
        }
    }
    return 0;
}

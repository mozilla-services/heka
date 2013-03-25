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
#include "lua_sandbox.h"
#include "_cgo_export.h"

////////////////////////////////////////////////////////////////////////////////
// private interface
////////////////////////////////////////////////////////////////////////////////
#define ERROR_SIZE 255
#define OUTPUT_SIZE 1024 * 4
#define MAX_MEMORY 1024 * 1024 * 8
#define MAX_INSTRUCTIONS 1000000

typedef struct
{
    size_t m_size;
    size_t m_pos;
    char*  m_data;
} output_data;

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

    output_data     m_output;

    char*           m_lua_file;
};

typedef struct
{
    const void*     m_ptr;
    size_t          m_name_pos;
} table_ref;

typedef struct
{
    size_t      m_size;
    size_t      m_pos;
    table_ref*  m_array;
} table_ref_array;

typedef struct
{
    FILE*           m_fh;
    output_data     m_keys;
    table_ref_array m_tables;
    const void*     m_globals;
} serialization_data;

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
int load_lua_libraries(lua_sandbox* lsb)
{
    // load base
    lua_pushcfunction(lsb->m_lua, luaopen_base);
    lua_pushstring(lsb->m_lua, "");
    lua_call(lsb->m_lua, 1, 0);
    // disable the base functions
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "collectgarbage");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "coroutine");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "dofile");
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
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "module");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "print");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawequal");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawget");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "rawset");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "require");
    lua_pushnil(lsb->m_lua);
    lua_setfield(lsb->m_lua, LUA_GLOBALSINDEX, "setfenv");

    // load math
    lua_pushcfunction(lsb->m_lua, luaopen_math);
    lua_pushstring(lsb->m_lua, LUA_MATHLIBNAME);
    lua_call(lsb->m_lua, 1, 0);
    lua_getglobal(lsb->m_lua, LUA_MATHLIBNAME);
    lua_newtable(lsb->m_lua);
    lua_setmetatable(lsb->m_lua, -2);
    lua_pop(lsb->m_lua, 1);

    // load os
    lua_pushcfunction(lsb->m_lua, luaopen_os);
    lua_pushstring(lsb->m_lua, LUA_OSLIBNAME);
    lua_call(lsb->m_lua, 1, 0);

    // disable the os functions
    lua_getglobal(lsb->m_lua, LUA_OSLIBNAME);
    if (!lua_istable(lsb->m_lua, 1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "cannot disable the os functions");
        sandbox_terminate(lsb);
        return 1;
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
    lua_newtable(lsb->m_lua);
    lua_setmetatable(lsb->m_lua, -2);
    lua_pop(lsb->m_lua, 1); // remove the os table

    // load string
    lua_pushcfunction(lsb->m_lua, luaopen_string);
    lua_pushstring(lsb->m_lua, LUA_STRLIBNAME);
    lua_call(lsb->m_lua, 1, 0);
    lua_getglobal(lsb->m_lua, LUA_STRLIBNAME);
    lua_newtable(lsb->m_lua);
    lua_setmetatable(lsb->m_lua, -2);
    lua_pop(lsb->m_lua, 1);

    // load table
    lua_pushcfunction(lsb->m_lua, luaopen_table);
    lua_pushstring(lsb->m_lua, LUA_TABLIBNAME);
    lua_call(lsb->m_lua, 1, 0);
    lua_getglobal(lsb->m_lua, LUA_TABLIBNAME);
    lua_newtable(lsb->m_lua);
    lua_setmetatable(lsb->m_lua, -2);
    lua_pop(lsb->m_lua, 1);

    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int dynamic_snprintf(output_data* output, const char* fmt, ...)
{
    va_list args;
    int result = 0;
    size_t len = 0, remaining = 0;
    char* ptr = NULL, *old_ptr = NULL;
    do {
        ptr = output->m_data + output->m_pos;
        remaining = output->m_size - output->m_pos;
        va_start(args, fmt);
        int len = vsnprintf(ptr, remaining, fmt, args);
        va_end(args);
        if (len >= remaining) {
            size_t newsize = output->m_size;
            while (len >= newsize - output->m_pos) {
                newsize *= 2;
            }
            void* p = malloc(newsize);
            if (p != NULL) {
                memcpy(p, output->m_data, output->m_pos);
                old_ptr = output->m_data;
                output->m_data = p;
                output->m_size = newsize;
            } else {
                return 1; // out of memory
            }
        } else {
            output->m_pos += len;
            break;
        }
    }
    while (1);
    free(old_ptr);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_data(lua_sandbox* lsb, int index, output_data* output)
{
    output->m_pos = 0;
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TNUMBER:
        if (dynamic_snprintf(output, "%0.9g",
                             lua_tonumber(lsb->m_lua, index))) {
            return 1;
        }
        break;
    case LUA_TSTRING:
        lua_checkstack(lsb->m_lua, 4);
        lua_getglobal(lsb->m_lua, "string");
        if (!lua_istable(lsb->m_lua, -1)) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data cannot access the string table");
            return 1;
        }
        lua_getfield(lsb->m_lua, -1, "format");
        if (!lua_isfunction(lsb->m_lua, -1)) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data cannot access the string format function");
            return 1;
        }
        lua_pushstring(lsb->m_lua, "%q");
        lua_pushvalue(lsb->m_lua, index - 3);
        if (lua_pcall(lsb->m_lua, 2, 1, 0) == 0) {
            if (dynamic_snprintf(output, "%s", lua_tostring(lsb->m_lua, -1))) {
                return 1;
            }
        } else {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data '%s'", lua_tostring(lsb->m_lua, -1));
            return 1;
        }
        lua_pop(lsb->m_lua, 2);
        break;
    case LUA_TNIL:
        if (dynamic_snprintf(output, "nil")) {
            return 1;
        }
        break;
    case LUA_TBOOLEAN:
        if (dynamic_snprintf(output, "%s",
                             lua_toboolean(lsb->m_lua, index)
                             ? "true" : "false")) {
            return 1;
        }
        break;
    default:
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "serialize_data cannot preserve type '%s'",
                 lua_typename(lsb->m_lua, lua_type(lsb->m_lua, index)));
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
table_ref* find_table_ref(table_ref_array* tra, const void* ptr)
{
    for (size_t i = 0; i < tra->m_pos; ++i) {
        if (ptr == tra->m_array[i].m_ptr) {
            return &tra->m_array[i];
        }
    }
    return NULL;
}

////////////////////////////////////////////////////////////////////////////////
table_ref* add_table_ref(table_ref_array* tra, const void* ptr, size_t name_pos)
{
    if (tra->m_pos == tra->m_size) {
        size_t newsize =  tra->m_size * 2;
        void* p = realloc(tra->m_array, newsize);
        if (p != NULL) {
            tra->m_array = p;
            tra->m_size = newsize;
        } else {
            return NULL;
        }
    }
    tra->m_array[tra->m_pos].m_ptr = ptr;
    tra->m_array[tra->m_pos].m_name_pos = name_pos;
    return &tra->m_array[tra->m_pos++];
}

int write_kvp(lua_sandbox* lsb, serialization_data* data, size_t parent);
////////////////////////////////////////////////////////////////////////////////
int serialize_table(lua_sandbox* lsb, serialization_data* data, size_t parent)
{
    int result = 0;
    lua_checkstack(lsb->m_lua, 2);
    lua_pushnil(lsb->m_lua);
    while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
        result = write_kvp(lsb, data, parent);
        lua_pop(lsb->m_lua, 1);
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int ignore_value_type(lua_sandbox* lsb, serialization_data* data, int index)
{
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TTABLE:
        if (lua_getmetatable(lsb->m_lua, index) != 0) {
            lua_pop(lsb->m_lua, 1);
            return 1;
        }
        if (lua_topointer(lsb->m_lua, index) == data->m_globals) {
            return 1;
        }
        break;
    case LUA_TNONE:
    case LUA_TUSERDATA:
    case LUA_TFUNCTION:
    case LUA_TTHREAD:
    case LUA_TLIGHTUSERDATA:
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int write_kvp(lua_sandbox* lsb, serialization_data* data, size_t parent)
{
    int kindex = -2, vindex = -1;

    if (ignore_value_type(lsb, data, vindex)) return 0;
    int result = serialize_data(lsb, kindex, &lsb->m_output);
    if (result != 0) return result;

    size_t pos = data->m_keys.m_pos;
    if (dynamic_snprintf(&data->m_keys, "%s[%s]", data->m_keys.m_data + parent,
                         lsb->m_output.m_data)) {
        return 1;
    }

    fprintf(data->m_fh, "%s = ", data->m_keys.m_data + pos);
    if (lua_type(lsb->m_lua, vindex) == LUA_TTABLE) {
        const void* ptr = lua_topointer(lsb->m_lua, vindex);
        table_ref* seen = find_table_ref(&data->m_tables, ptr);
        if (seen == NULL) {
            seen = add_table_ref(&data->m_tables, ptr, pos);
            if (seen != NULL) {
                data->m_keys.m_pos += 1;
                fprintf(data->m_fh, "{}\n");
                result = serialize_table(lsb, data, pos);
            } else {
                snprintf(lsb->m_error_message, ERROR_SIZE,
                         "preserve table out of memory");
                return 1;
            }
        } else {
            data->m_keys.m_pos = pos;
            fprintf(data->m_fh, "%s\n", data->m_keys.m_data + seen->m_name_pos);
        }
    } else {
        data->m_keys.m_pos = pos;
        result = serialize_data(lsb, vindex, &lsb->m_output);
        if (result == 0) {
            fprintf(data->m_fh, "%s\n", lsb->m_output.m_data);
        }
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int preserve_global_data(lua_sandbox* lsb, const char* data_file)
{
    static const char* G = "_G";
    lua_getglobal(lsb->m_lua, G);
    if (!lua_istable(lsb->m_lua, -1)) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "preserve_global_data cannot access the global table");
        return 1;
    }

    int result = 0;
    FILE* fh = fopen(data_file, "wb");
    if (fh == NULL) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "preserve_global_data could not open: %s", data_file);
        result = 1;
    } else {
        serialization_data data;
        data.m_fh = fh;
        data.m_keys.m_size = OUTPUT_SIZE;
        data.m_keys.m_pos = 0;
        data.m_keys.m_data = malloc(data.m_keys.m_size);
        data.m_tables.m_size = 64;
        data.m_tables.m_pos = 0;
        data.m_tables.m_array = malloc(data.m_tables.m_size * sizeof(table_ref));
        if (data.m_tables.m_array == NULL || data.m_keys.m_data == NULL) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "preserve_global_data out of memory");
            result = 1;
        } else {
            dynamic_snprintf(&data.m_keys, "%s", G);
            data.m_keys.m_pos += 1;
            data.m_globals = lua_topointer(lsb->m_lua, -1);
            lua_checkstack(lsb->m_lua, 2);
            lua_pushnil(lsb->m_lua);
            while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
                result = write_kvp(lsb, &data, 0);
                lua_pop(lsb->m_lua, 1);
            }
            lua_pop(lsb->m_lua, lua_gettop(lsb->m_lua));
        }
        free(data.m_tables.m_array);
        free(data.m_keys.m_data);
        fclose(fh);
    }
    if (result != 0) {
        remove(data_file);
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int restore_global_data(lua_sandbox* lsb, const char* data_file)
{
    unsigned configured_memory =  lsb->m_memory_limit;
    unsigned configured_instructions =  lsb->m_memory_limit;
    // increase the sandbox limits during restoration
    lsb->m_memory_limit = MAX_MEMORY * 2;
    lua_sethook(lsb->m_lua, sandbox_instruction_manager, LUA_MASKCOUNT,
                MAX_INSTRUCTIONS * 10);

    if (luaL_dofile(lsb->m_lua, data_file) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "restore_global_data %s",
                 lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 2;
    } else {
        lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
        lsb->m_memory_limit = configured_memory;
        lsb->m_instruction_limit = configured_instructions;
        lsb->m_memory_usage_maximum = lsb->m_memory_usage_current;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
// private go callbacks
////////////////////////////////////////////////////////////////////////////////
int sandbox_read_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "read_message() invalid lightuserdata");
        lua_error(lua);
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    const char* field;
    int fi = 0, ai = 0;
    switch (lua_gettop(lua)) {
    case 3:
        ai = luaL_checkint(lua, 3);
        if (ai < 0) {
            lua_pushstring(lua, "read_message() array index must be >= 0");
            lua_error(lua);
        }
        // fall-thru
    case 2:
        fi = luaL_checkint(lua, 2);
        if (fi < 0) {
            lua_pushstring(lua, "read_message() field index must be >= 0");
            lua_error(lua);
        }
        // fall-thru
    case 1:
        field = luaL_checkstring(lua, 1);
        break;
    default:
        lua_pushstring(lua, "read_message() incorrect number of arguments");
        lua_error(lua);
        break;
    }

    struct go_lua_read_message_return gr;
    // cast away constness, the value is not modified and it will save a copy
    gr = go_lua_read_message(lsb->m_go, (char*)field, fi, ai);
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
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_output(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "output() invalid lightuserdata");
        lua_error(lua);
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (n == 0) {
        lua_pushstring(lua, "output() must have at least one argument");
        lua_error(lua);
    }
    
    int result = 0;
    for (int i = 1; result == 0 && i <= n; ++i) {
        switch (lua_type(lua, i)) {
        case LUA_TNUMBER:
            if (dynamic_snprintf(&lsb->m_output, "%0.9g", 
                                 lua_tonumber(lua, i))) {
                result = 1;
            }
            break;
        case LUA_TSTRING:
            if (dynamic_snprintf(&lsb->m_output, "%s", lua_tostring(lua, i))) {
                result = 1;
            }
            break;
        case LUA_TNIL:
            if (dynamic_snprintf(&lsb->m_output, "nil")) {
                result = 1;
            }
            break;
        default:
            // ignore other types
            break;
        }
    }
    if (result != 0) {
        lua_pushstring(lua, "output() out of memory");
        lua_error(lua);
    }

    go_lua_output(lsb->m_go, lsb->m_output.m_data);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int sandbox_inject_message(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        lua_pushstring(lua, "inject_message() invalid lightuserdata");
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
            go_lua_inject_message(lsb->m_go, (char*)msg);
        }  else {
            lua_pushstring(lua, "inject_message() argument must be a string");
            lua_error(lua);
        }
    } else {
        lua_pushstring(lua, "inject_message() incorrect number of arguments");
        lua_error(lua);
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
// public interface
////////////////////////////////////////////////////////////////////////////////
lua_sandbox* lua_sandbox_create(void* go,
                                const char* lua_file,
                                unsigned mem_limit,
                                unsigned inst_limit)
{
    if (mem_limit > MAX_MEMORY || inst_limit > MAX_INSTRUCTIONS) {
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
        lsb->m_output.m_pos = 0;
        lsb->m_output.m_size = OUTPUT_SIZE;
        lsb->m_output.m_data = malloc(lsb->m_output.m_size);
        size_t len = strlen(lua_file);
        lsb->m_lua_file = malloc(len + 1);
        lsb->m_lua_file[len] = 0;
        memcpy(lsb->m_lua_file, lua_file, len);
    }
    return lsb;
}

////////////////////////////////////////////////////////////////////////////////
char* lua_sandbox_destroy(lua_sandbox* lsb, const char* data_file)
{
    char* err = NULL;
    if (lsb != NULL) {
        if (data_file != NULL && strnlen(data_file, 1) > 0) {
            if (preserve_global_data(lsb, data_file) != 0) {
                size_t len = strnlen(lsb->m_error_message, ERROR_SIZE);
                err = malloc(len+1);
                if (err != NULL) {
                    memcpy(err, lsb->m_error_message, len+1);
                }
            }
        }
        sandbox_terminate(lsb);
        free(lsb->m_output.m_data);
        free(lsb->m_lua_file);
        free(lsb);
    }
    return err;
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
    lsb->m_instruction_usage_current = sandbox_instruction_usage(lsb);
    if (lsb->m_instruction_usage_current > lsb->m_instruction_usage_maximum) {
        lsb->m_instruction_usage_maximum = lsb->m_instruction_usage_current;
    }
    return status;
}

////////////////////////////////////////////////////////////////////////////////
int lua_sandbox_timer_event(lua_sandbox* lsb, long long ns)
{
    if (lsb == NULL || lsb->m_lua == NULL) {
        return 1;
    }

    lua_sethook(lsb->m_lua, sandbox_instruction_manager, LUA_MASKCOUNT,
                lsb->m_instruction_limit);
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
    lsb->m_instruction_usage_current = sandbox_instruction_usage(lsb);
    if (lsb->m_instruction_usage_current > lsb->m_instruction_usage_maximum) {
        lsb->m_instruction_usage_maximum = lsb->m_instruction_usage_current;
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
    lsb->m_lua = lua_newstate(sandbox_memory_manager, lsb);
    if (lsb->m_lua != NULL) {
        if (load_lua_libraries(lsb) != 0) {
            return 2;
        }

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_read_message, 1);
        lua_setglobal(lsb->m_lua, "read_message");

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_output, 1);
        lua_setglobal(lsb->m_lua, "output");

        lua_pushlightuserdata(lsb->m_lua, (void*)lsb);
        lua_pushcclosure(lsb->m_lua, &sandbox_inject_message, 1);
        lua_setglobal(lsb->m_lua, "inject_message");
        lua_sethook(lsb->m_lua, sandbox_instruction_manager, LUA_MASKCOUNT,
                    lsb->m_instruction_limit);

        if (luaL_dofile(lsb->m_lua, lsb->m_lua_file) != 0) {
            snprintf(lsb->m_error_message, ERROR_SIZE, "%s",
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
            if (data_file != NULL && strnlen(data_file, 1) > 0) {
                return restore_global_data(lsb, data_file);
            }
        }
    } else {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "lua_newstate out of memory");
        sandbox_terminate(lsb);
        return 3;
    }
    return 0;
}

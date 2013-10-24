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
#include "lua_sandbox_private.h"
#include "lua_sandbox_protobuf.h"
#include "lua_circular_buffer.h"
#include "_cgo_export.h"

////////////////////////////////////////////////////////////////////////////////
void load_library(lua_State* lua, const char* table, lua_CFunction f,
                  const char** disable)
{
    lua_pushcfunction(lua, f);
    lua_call(lua, 0, 1);

    if (strlen(table) == 0) { // Handle the special "" base table.
        for (int i = 0; disable[i] != NULL; ++i) {
            lua_pushnil(lua);
            lua_setfield(lua, LUA_GLOBALSINDEX, disable[i]);
        }
    } else {
        for (int i = 0; disable[i] != NULL; ++i) {
            lua_pushnil(lua);
            lua_setfield(lua, -2, disable[i]);
        }
        // Add an empty metatable to identify core libraries during
        // preservation.
        lua_newtable(lua);
        lua_setmetatable(lua, -2);
    }
}

////////////////////////////////////////////////////////////////////////////////
void* memory_manager(void* ud, void* ptr, size_t osize, size_t nsize)
{
    lua_sandbox* lsb = (lua_sandbox*)ud;

    void* nptr = NULL;
    if (nsize == 0) {
        free(ptr);
        lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT] -= osize;
    } else {
        unsigned new_state_memory =
          lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT] + nsize - osize;
        if (new_state_memory
            <= lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_LIMIT]) {
            nptr = realloc(ptr, nsize);
            if (nptr != NULL) {
                lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT] =
                  new_state_memory;
                if (lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT]
                    > lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_MAXIMUM]) {
                    lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_MAXIMUM] =
                      lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT];
                }
            }
        }
    }
    return nptr;
}

////////////////////////////////////////////////////////////////////////////////
void instruction_manager(lua_State* lua, lua_Debug* ar)
{
    if (LUA_HOOKCOUNT == ar->event) {
        luaL_error(lua, "instruction_limit exceeded");
    }
}

////////////////////////////////////////////////////////////////////////////////
size_t instruction_usage(lua_sandbox* lsb)
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
    lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT] = 0;
    lsb->m_status = STATUS_TERMINATED;
}

////////////////////////////////////////////////////////////////////////////////
void update_output_stats(lua_sandbox* lsb)
{
    lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_CURRENT] = lsb->m_output.m_pos;
    if (lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_CURRENT]
        > lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_MAXIMUM]) {
        lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_MAXIMUM] =
          lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_CURRENT];
    }
}

////////////////////////////////////////////////////////////////////////////////
int realloc_output(output_data* output, size_t needed)
{
    if (needed + output->m_pos > MAX_OUTPUT) return 1;
    size_t newsize = output->m_size * 2;
    while (needed >= newsize - output->m_pos) {
        newsize *= 2;
    }

    void* ptr = realloc(output->m_data, newsize);
    if (ptr == NULL) return 1;
    output->m_data = ptr;
    output->m_size = newsize;
    return 0;
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

    FILE* fh = fopen(data_file, "wb");
    if (fh == NULL) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "preserve_global_data could not open: %s", data_file);
        return 1;
    }

    int result = 0;
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
            result = serialize_kvp(lsb, &data, 0);
            lua_pop(lsb->m_lua, 1);
        }
        lua_pop(lsb->m_lua, lua_gettop(lsb->m_lua));
        // Wipe the entire Lua stack.  Since incremental cleanup on failure
        // was added the stack should only contain table _G.
    }
    free(data.m_tables.m_array);
    free(data.m_keys.m_data);
    fclose(fh);
    if (result != 0) {
        remove(data_file);
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_double(output_data* output, double d)
{
    if (d > INT_MAX) {
        return dynamic_snprintf(output, "%0.9g", d);
    }

    const int precision = 8;
    const unsigned magnitude = 100000000;
    char buffer[20];
    char* p = buffer;
    int negative = 0;

    if (d < 0) {
        negative = 1;
        d = -d;
    }

    int number = (int)d;
    double tmp = (d - number) * magnitude;
    unsigned fraction = (unsigned)tmp;
    double diff = tmp - fraction;

    if (diff > 0.5) {
        ++fraction;
        if (fraction >= magnitude) {
            fraction = 0;
            ++number;
        }
    } else if (diff == 0.5 && ((fraction == 0) || (fraction & 1))) {
        // bankers rounding
        ++fraction;
    }

    // output decimal fraction
    if (fraction != 0) {
        int nodigits = 1;
        char c = 0;
        for (int x = 0; x < precision; ++x) {
            c = fraction % 10;
            if (!(c == 0 && nodigits)) {
                *p++ = c + '0';
                nodigits = 0;
            }
            fraction /= 10;
        }
        *p++ = '.';
    }

    // output number
    do {
        *p++ = (number % 10) + '0';
        number /= 10;
    }
    while (number > 0);

    size_t remaining = output->m_size - output->m_pos;
    size_t len = (p - buffer) + negative;
    if (len >= remaining) {
        size_t newsize = output->m_size * 2;
        while (len >= newsize - output->m_pos) {
            newsize *= 2;
        }
        void* ptr = realloc(output->m_data, newsize);
        if (ptr == NULL) return 1;
        output->m_data = ptr;
        output->m_size = newsize;
    }

    if (negative) {
        output->m_data[output->m_pos++] = '-';
    }
    do {
        --p;
        output->m_data[output->m_pos++] = *p;
    }
    while (p != buffer);
    output->m_data[output->m_pos] = 0;

    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_table(lua_sandbox* lsb, serialization_data* data, size_t parent)
{
    int result = 0;
    lua_checkstack(lsb->m_lua, 2);
    lua_pushnil(lsb->m_lua);
    while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
        result = serialize_kvp(lsb, data, parent);
        lua_pop(lsb->m_lua, 1); // Remove the value leaving the key on top for
                                // the next interation.
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_table_as_json(lua_sandbox* lsb,
                            serialization_data* data,
                            int isHash)
{
    int result = 0;
    lua_checkstack(lsb->m_lua, 2);
    lua_pushnil(lsb->m_lua);
    int had_output = 0;
    size_t start = 0;
    while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
        if (had_output) {
            if (dynamic_snprintf(&lsb->m_output, ",")) return 1;
        }
        start = lsb->m_output.m_pos;
        result = serialize_kvp_as_json(lsb, data, isHash);
        lua_pop(lsb->m_lua, 1); // Remove the value leaving the key on top for
                                // the next interation.
        if (start != lsb->m_output.m_pos) {
            had_output = 1;
        } else {
            had_output = 0;
        }
    }
    if (start != 0 && had_output == 0) { // remove the trailing comma
        size_t reset_pos = start - 1;
        if (lsb->m_output.m_data[reset_pos] == ',') {
            lsb->m_output.m_data[reset_pos] = 0;
            lsb->m_output.m_pos = reset_pos;
        }
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_table_as_pb(lua_sandbox* lsb)
{
    output_data* d = &lsb->m_output;
    d->m_pos = 0;
    size_t needed = 18;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    // create a type 4 uuid
    d->m_data[d->m_pos++] = 2 | (1 << 3);
    d->m_data[d->m_pos++] = 16;
    for (int x = 0; x < 16; ++x) {
        d->m_data[d->m_pos++] = rand() % 255;
    }
    d->m_data[8] = (d->m_data[8] & 0x0F) | 0x40;
    d->m_data[10] = (d->m_data[10] & 0x0F) | 0xA0;

    // use existing or create a timestamp
    lua_getfield(lsb->m_lua, 1, "Timestamp");
    long long ts;
    if (lua_isnumber(lsb->m_lua, -1)) {
        ts = (long long)lua_tonumber(lsb->m_lua, -1);
    } else {
        ts = time(NULL) * 1e9;
    }
    lua_pop(lsb->m_lua, 1);
    if (pb_write_tag(d, 2, 0)) return 1;
    if (pb_write_varint(d, ts)) return 1;

    if (encode_string(lsb, d, 3, "Type")) return 1;
    if (encode_string(lsb, d, 4, "Logger")) return 1;
    if (encode_int(lsb, d, 5, "Severity")) return 1;
    if (encode_string(lsb, d, 6, "Payload")) return 1;
    if (encode_string(lsb, d, 7, "EnvVersion")) return 1;
    if (encode_int(lsb, d, 8, "Pid")) return 1;
    if (encode_string(lsb, d, 9, "Hostname")) return 1;
    if (encode_fields(lsb, d, 10, "Fields")) return 1;
    // if we go above 15 pb_write_tag will need to start varint encoding
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_data(lua_sandbox* lsb, int index, output_data* output)
{
    output->m_pos = 0;
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TNUMBER:
        if (serialize_double(output, lua_tonumber(lsb->m_lua, index))) {
            return 1;
        }
        break;
    case LUA_TSTRING:
        // The stack is cleaned up on failure by preserve_global_data
        // but for clarity it is incrementally cleaned up anyway.
        lua_checkstack(lsb->m_lua, 4);
        lua_getglobal(lsb->m_lua, "string");
        if (!lua_istable(lsb->m_lua, -1)) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data cannot access the string table");
            lua_pop(lsb->m_lua, 1); // Remove bogus string table.
            return 1;
        }
        lua_getfield(lsb->m_lua, -1, "format");
        if (!lua_isfunction(lsb->m_lua, -1)) {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data cannot access the string format function");
            lua_pop(lsb->m_lua, 2); // Remove the bogus format function and
                                    // string table.
            return 1;
        }
        lua_pushstring(lsb->m_lua, "%q");
        lua_pushvalue(lsb->m_lua, index - 3);
        if (lua_pcall(lsb->m_lua, 2, 1, 0) == 0) {
            if (dynamic_snprintf(output, "%s", lua_tostring(lsb->m_lua, -1))) {
                lua_pop(lsb->m_lua, 1); // Remove the string table.
                return 1;
            }
        } else {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "serialize_data '%s'", lua_tostring(lsb->m_lua, -1));
            lua_pop(lsb->m_lua, 2); // Remove the error message and the string
                                    // table.
            return 1;
        }
        lua_pop(lsb->m_lua, 2); // Remove the pcall result and the string table.
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
int serialize_data_as_json(lua_sandbox* lsb, int index, output_data* output)
{
    const char* s;
    size_t len = 0;
    size_t start_pos = output->m_pos;
    size_t escaped_len = 0;
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TNUMBER:
        if (serialize_double(output, lua_tonumber(lsb->m_lua, index))) {
            return 1;
        }
        break;
    case LUA_TSTRING:
        s = lua_tolstring(lsb->m_lua, index, &len);
        escaped_len = len + 3; // account for the quotes and terminator
        for (size_t i = 0; i < len; ++i) {
            // buffer needs at least enough room for quotes, terminator, and an
            // escaped character
            if (output->m_pos + 5 > output->m_size) {
                size_t needed = escaped_len - (output->m_pos - start_pos);
                if (realloc_output(output, needed)) return 1;
            }
            if (i == 0) {
                output->m_data[output->m_pos++] = '"';
            }
            switch (s[i]) {
            case '"':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = '"';
                ++escaped_len;
                break;
            case '\\':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = '\\';
                ++escaped_len;
                break;
            case '/':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = '/';
                ++escaped_len;
                break;
            case '\b':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = 'b';
                ++escaped_len;
                break;
            case '\f':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = 'f';
                ++escaped_len;
                break;
            case '\n':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = 'n';
                ++escaped_len;
                break;
            case '\r':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = 'r';
                ++escaped_len;
                break;
            case '\t':
                output->m_data[output->m_pos++] = '\\';
                output->m_data[output->m_pos++] = 't';
                ++escaped_len;
                break;
            default:
                output->m_data[output->m_pos++] = s[i];
            }
        }
        output->m_data[output->m_pos++] = '"';
        output->m_data[output->m_pos] = 0;
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
                 "serialize_data_as_json cannot preserve type '%s'",
                 lua_typename(lsb->m_lua, lua_type(lsb->m_lua, index)));
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
const char* userdata_type(lua_State* lua, void* ud, int index)
{
    const char* table = NULL;
    if (ud == NULL) return table;

    if (lua_getmetatable(lua, index)) {
        lua_getfield(lua, LUA_REGISTRYINDEX, heka_circular_buffer);
        if (lua_rawequal(lua, -1, -2)) {
            table = heka_circular_buffer;
        }
    }
    lua_pop(lua, 2); // metatable and field
    return table;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_kvp(lua_sandbox* lsb, serialization_data* data, size_t parent)
{
    int kindex = -2, vindex = -1, result = 0;

    if (ignore_value_type(lsb, data, vindex)) return 0;
    if (serialize_data(lsb, kindex, &lsb->m_output)) return 1;

    size_t pos = data->m_keys.m_pos;
    if (dynamic_snprintf(&data->m_keys, "%s[%s]", data->m_keys.m_data + parent,
                         lsb->m_output.m_data)) {
        return 1;
    }

    if (lua_type(lsb->m_lua, vindex) == LUA_TTABLE) {
        const void* ptr = lua_topointer(lsb->m_lua, vindex);
        table_ref* seen = find_table_ref(&data->m_tables, ptr);
        if (seen == NULL) {
            seen = add_table_ref(&data->m_tables, ptr, pos);
            if (seen != NULL) {
                data->m_keys.m_pos += 1;
                fprintf(data->m_fh, "%s = {}\n", data->m_keys.m_data + pos);
                result = serialize_table(lsb, data, pos);
            } else {
                snprintf(lsb->m_error_message, ERROR_SIZE,
                         "preserve table out of memory");
                return 1;
            }
        } else {
            fprintf(data->m_fh, "%s = ", data->m_keys.m_data + pos);
            data->m_keys.m_pos = pos;
            fprintf(data->m_fh, "%s\n", data->m_keys.m_data + seen->m_name_pos);
        }
    } else if (lua_type(lsb->m_lua, vindex) == LUA_TUSERDATA) {
        void* ud = lua_touserdata(lsb->m_lua, vindex);
        if (heka_circular_buffer == userdata_type(lsb->m_lua, ud, vindex)) {
            table_ref* seen = find_table_ref(&data->m_tables, ud);
            if (seen == NULL) {
                seen = add_table_ref(&data->m_tables, ud, pos);
                if (seen != NULL) {
                    data->m_keys.m_pos += 1;
                    result = serialize_circular_buffer(lsb->m_lua,
                      data->m_keys.m_data + pos,
                      (circular_buffer*)ud, &lsb->m_output);
                    if (result == 0) {
                        fprintf(data->m_fh, "%s", lsb->m_output.m_data);
                    }
                } else {
                    snprintf(lsb->m_error_message, ERROR_SIZE,
                             "preserve table out of memory");
                    return 1;
                }
            } else {
                fprintf(data->m_fh, "%s = ", data->m_keys.m_data + pos);
                data->m_keys.m_pos = pos;
                fprintf(data->m_fh, "%s\n", data->m_keys.m_data +
                        seen->m_name_pos);
            }
        }
    } else {
        fprintf(data->m_fh, "%s = ", data->m_keys.m_data + pos);
        data->m_keys.m_pos = pos;
        result = serialize_data(lsb, vindex, &lsb->m_output);
        if (result == 0) {
            fprintf(data->m_fh, "%s\n", lsb->m_output.m_data);
        }
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int ignore_key(lua_sandbox* lsb, int index)
{
    if (lua_type(lsb->m_lua, index) == LUA_TSTRING) {
        const char* key = lua_tostring(lsb->m_lua, index);
        if (key[0] == '_') {
            return 1;
        }
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_kvp_as_json(lua_sandbox* lsb,
                          serialization_data* data,
                          int isHash)
{
    static const char* array_start = "[", *array_end = "]";
    static const char* hash_start = "{", *hash_end = "}";
    int kindex = -2, vindex = -1, result = 0;

    if (ignore_value_type_json(lsb, vindex)) return 0;
    if (ignore_key(lsb, kindex)) return 0;
    if (isHash) {
        if (serialize_data_as_json(lsb, kindex, &lsb->m_output)) return 1;
        if (dynamic_snprintf(&lsb->m_output, ":")) return 1;
    }

    if (lua_type(lsb->m_lua, vindex) == LUA_TTABLE) {
        const void* ptr = lua_topointer(lsb->m_lua, vindex);
        table_ref* seen = find_table_ref(&data->m_tables, ptr);
        if (seen == NULL) {
            seen = add_table_ref(&data->m_tables, ptr, 0);
            if (seen != NULL) {
                const char* start, *end;
                lua_rawgeti(lsb->m_lua, vindex, 1);
                int hash = lua_isnil(lsb->m_lua, -1);
                lua_pop(lsb->m_lua, 1); // remove the test value
                if (hash) {
                    start = hash_start;
                    end = hash_end;
                } else {
                    start = array_start;
                    end = array_end;
                }
                if (dynamic_snprintf(&lsb->m_output, start)) return 1;
                if (serialize_table_as_json(lsb, data, hash)) return 1;
                if (dynamic_snprintf(&lsb->m_output, end)) return 1;
            } else {
                snprintf(lsb->m_error_message, ERROR_SIZE,
                         "serialize table out of memory");
                return 1;
            }
        } else {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "table contains an internal or circular reference");
            return 1;
        }
    } else {
        result = serialize_data_as_json(lsb, vindex, &lsb->m_output);
    }
    return result;
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
        void* p = realloc(tra->m_array, newsize * sizeof(table_ref));
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

////////////////////////////////////////////////////////////////////////////////
int ignore_value_type(lua_sandbox* lsb, serialization_data* data, int index)
{
    void* ud = NULL;
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TTABLE:
        if (lua_getmetatable(lsb->m_lua, index) != 0) {
            lua_pop(lsb->m_lua, 1); // Remove the metatable.
            return 1;
        }
        if (lua_topointer(lsb->m_lua, index) == data->m_globals) {
            return 1;
        }
        break;
    case LUA_TUSERDATA:
        ud = lua_touserdata(lsb->m_lua, index);
        if ((heka_circular_buffer != userdata_type(lsb->m_lua, ud, index))) {
            return 1;
        }
        break;
    case LUA_TNONE:
    case LUA_TFUNCTION:
    case LUA_TTHREAD:
    case LUA_TLIGHTUSERDATA:
    case LUA_TNIL:
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int ignore_value_type_json(lua_sandbox* lsb, int index)
{
    switch (lua_type(lsb->m_lua, index)) {
    case LUA_TTABLE:
        if (lua_getmetatable(lsb->m_lua, index) != 0) {
            lua_pop(lsb->m_lua, 1); // Remove the metatable.
            return 1;
        }
        break;
    case LUA_TUSERDATA:
    case LUA_TNONE:
    case LUA_TFUNCTION:
    case LUA_TTHREAD:
    case LUA_TLIGHTUSERDATA:
    case LUA_TNIL:
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int restore_global_data(lua_sandbox* lsb, const char* data_file)
{
    unsigned configured_memory =
      lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_LIMIT];
    // Increase the sandbox limits during restoration.
    lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_LIMIT] = MAX_MEMORY * 2;
    // Clear the sandbox instruction limit hook.
    lua_sethook(lsb->m_lua, instruction_manager, 0, 0);

    if (luaL_dofile(lsb->m_lua, data_file) != 0) {
        snprintf(lsb->m_error_message, ERROR_SIZE,
                 "restore_global_data %s",
                 lua_tostring(lsb->m_lua, -1));
        sandbox_terminate(lsb);
        return 2;
    } else {
        lua_gc(lsb->m_lua, LUA_GCCOLLECT, 0);
        lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_LIMIT] = configured_memory;
        lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_MAXIMUM] =
          lsb->m_usage[USAGE_TYPE_MEMORY][USAGE_STAT_CURRENT];
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int dynamic_snprintf(output_data* output, const char* fmt, ...)
{
    va_list args;
    int result = 0;
    int remaining = 0;
    char* ptr = NULL, *old_ptr = NULL;
    do {
        ptr = output->m_data + output->m_pos;
        remaining = output->m_size - output->m_pos;
        va_start(args, fmt);
        int len = vsnprintf(ptr, remaining, fmt, args);
        va_end(args);
        if (len == -1) {
            // Windows and Unix have different return values for this function
            // -1 on Unix is a format error
            // -1 on Windows means the buffer is too small and the required len
            // is not returned
            len = remaining;
        }
        if (len >= remaining) {
            size_t newsize = output->m_size * 2;
            while ((size_t)len >= newsize - output->m_pos) {
                newsize *= 2;
            }
            void* p = malloc(newsize);
            if (p != NULL) {
                memcpy(p, output->m_data, output->m_pos);
                old_ptr = output->m_data;
                output->m_data = p;
                output->m_size = newsize;
            } else {
                return 1; // Out of memory condition.
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
int output(lua_State* lua)
{
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "output() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    int n = lua_gettop(lua);
    if (n == 0) {
        luaL_error(lua, "output() must have at least one argument");
    }

    int result = 0;
    void* ud = NULL;
    for (int i = 1; result == 0 && i <= n; ++i) {
        switch (lua_type(lua, i)) {
        case LUA_TNUMBER:
            if (serialize_double(&lsb->m_output, lua_tonumber(lua, i))) {
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
        case LUA_TBOOLEAN:
            if (dynamic_snprintf(&lsb->m_output, "%s",
                                 lua_toboolean(lsb->m_lua, i)
                                 ? "true" : "false")) {
                result = 1;
            }
            break;
        case LUA_TTABLE:
            if (!dynamic_snprintf(&lsb->m_output, "{")) {
                serialization_data data;
                data.m_globals = NULL;
                data.m_tables.m_size = 64;
                data.m_tables.m_pos = 0;
                data.m_tables.m_array = malloc(data.m_tables.m_size * sizeof(table_ref));
                if (data.m_tables.m_array == NULL) {
                    snprintf(lsb->m_error_message, ERROR_SIZE,
                             "json table serialization out of memory");
                    result = 1;
                } else {
                    lua_checkstack(lsb->m_lua, 2);
                    lua_getfield(lsb->m_lua, i, "_name");
                    if (lua_type(lsb->m_lua, -1) != LUA_TSTRING) {
                        lua_pop(lsb->m_lua, 1); // remove the failed _name result
                        lua_pushstring(lsb->m_lua, "table"); // add default name
                    }
                    lua_pushvalue(lsb->m_lua, i);
                    result = serialize_kvp_as_json(lsb, &data, 1);
                    if (result == 0) {
                        result = dynamic_snprintf(&lsb->m_output, "}\n");
                    }
                    lua_pop(lsb->m_lua, 2); // remove the name and copy of the table
                    free(data.m_tables.m_array);
                }
            } else {
                result = 1;
            }
            break;
        case LUA_TUSERDATA:
            ud = lua_touserdata(lua, i);
            if (heka_circular_buffer == userdata_type(lua, ud, i)) {
                if (output_circular_buffer(lua, (circular_buffer*)ud,
                                           &lsb->m_output)) {
                    result = 1;
                }
            }
            break;
        }
    }
    update_output_stats(lsb);
    if (result != 0
        || lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_CURRENT]
        > lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_LIMIT]) {
        if (lsb->m_error_message[0] == 0) {
            luaL_error(lua, "output_limit exceeded");
        }
        luaL_error(lua, lsb->m_error_message);
    }
    return 0;
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
int inject_message(lua_State* lua)
{
    static const char* default_type = "txt";
    static const char* default_name = "";
    void* luserdata = lua_touserdata(lua, lua_upvalueindex(1));
    if (NULL == luserdata) {
        luaL_error(lua, "inject_message() invalid lightuserdata");
    }
    lua_sandbox* lsb = (lua_sandbox*)luserdata;

    void* ud = NULL;
    const char* type = default_type;
    const char* name = default_name;
    switch (lua_gettop(lua)) {
    case 0:
        break;
    case 2:
        name = luaL_checkstring(lua, 2);
        // fallthru
    case 1:
        switch (lua_type(lua, 1)) {
        case LUA_TSTRING:
            type = lua_tostring(lua, 1);
            if (strlen(type) == 0) type = default_type;
            break;
        case LUA_TTABLE:
            type = "";
            if (serialize_table_as_pb(lsb) != 0) {
                luaL_error(lua, "inject_message() cound not encode protobuf - %s",
                           lsb->m_error_message);
            }
            break;
        case LUA_TUSERDATA:
            ud = lua_touserdata(lua, 1);
            if (heka_circular_buffer == userdata_type(lua, ud, 1)) {
                circular_buffer* cb = (circular_buffer*)ud;
                type = get_output_format(cb);
                lsb->m_output.m_pos = 0;
                if (output_circular_buffer(lua, cb, &lsb->m_output)) {
                    luaL_error(lua, lsb->m_error_message);
                }
            } else {
                luaL_typerror(lua, 1, "circular_buffer");
            }
            break;
        default:
            luaL_typerror(lua, 1, "string, table, or circular_buffer");
            break;
        }
        break;
    default:
        luaL_error(lua, "inject_message() takes a maximum of 2 arguments");
        break;
    }

    if (lsb->m_output.m_pos != 0) {
        update_output_stats(lsb);
        if (lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_CURRENT]
            > lsb->m_usage[USAGE_TYPE_OUTPUT][USAGE_STAT_LIMIT]) {
            if (lsb->m_error_message[0] == 0) {
                luaL_error(lua, "output_limit exceeded");
            }
            luaL_error(lua, lsb->m_error_message);
        }
        int result = go_lua_inject_message(lsb->m_go,
                                           lsb->m_output.m_data,
                                           (int)(lsb->m_output.m_pos),
                                           (char*)type,
                                           (char*)name);
        lsb->m_output.m_pos = 0;
        if (result != 0) {
            luaL_error(lua, "inject_message() exceeded MaxMsgLoops");
        }
    }
    return 0;
}

LUALIB_API int (luaopen_cjson_safe) (lua_State *L);
////////////////////////////////////////////////////////////////////////////////
int require_library(lua_State* lua)
{
    const char* name = luaL_checkstring(lua, 1);
    if (strcmp(name, LUA_LPEGLIBNAME) == 0) {
        const char* disable[] = { NULL };
        load_library(lua, name, luaopen_lpeg, disable);
    } else if (strcmp(name, "cjson") == 0) {
        const char* disable[] = { "encode",  "encode_sparse_array", 
            "encode_max_depth", "encode_number_precision", "encode_keep_buffer",
            "encode_invalid_numbers", NULL};
        load_library(lua, name, luaopen_cjson_safe, disable);
        lua_pushvalue(lua, -1);
        lua_setglobal(lua, name);
    } else {
        luaL_error(lua, "library '%s' is not available", name);
    }
    return 1;
}

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
    gr = go_lua_read_config(lsb->m_go, (char*)name);
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

// todo split the protobuf code out to a separate source file when
// http://code.google.com/p/go/issues/detail?id=5740 is resolved
////////////////////////////////////////////////////////////////////////////////
int pb_write_varint(output_data* d, long long i)
{
    size_t needed = 10;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    if (i == 0) {
        d->m_data[d->m_pos++] = 0;
        return 0;
    }

    while (i) {
        d->m_data[d->m_pos++] = (i & 0x7F) | 0x80;
        i >>= 7;
    }
    d->m_data[d->m_pos - 1] &= 0x7F; // end the varint
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int pb_write_double(output_data* d, double i)
{
    size_t needed = sizeof(double);
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    memcpy(&d->m_data[d->m_pos], &i, needed);
    d->m_pos += needed;
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int pb_write_bool(output_data* d, int i)
{
    size_t needed = 1;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    if (i) {
        d->m_data[d->m_pos++] = 1;
    } else {
        d->m_data[d->m_pos++] = 0;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int pb_write_tag(output_data* d, char id, char wire_type)
{
    size_t needed = 1;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    d->m_data[d->m_pos++] = wire_type | (id << 3);
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int pb_write_string(output_data* d, char id, const char* s, size_t len)
{

    if (pb_write_tag(d, id, 2)) return 1;
    if (pb_write_varint(d, len)) return 1;

    size_t needed = len;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }
    memcpy(&d->m_data[d->m_pos], s, len);
    d->m_pos += len;
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int encode_string(lua_sandbox* lsb, output_data* d, char id, const char* name)
{
    int result = 0;
    lua_getfield(lsb->m_lua, 1, name);
    if (lua_isstring(lsb->m_lua, -1)) {
        size_t len;
        const char* s = lua_tolstring(lsb->m_lua, -1, &len);
        result = pb_write_string(d, id, s, len);
    }
    lua_pop(lsb->m_lua, 1);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int encode_int(lua_sandbox* lsb, output_data* d, char id, const char* name)
{
    int result = 0;
    lua_getfield(lsb->m_lua, 1, name);
    if (lua_isnumber(lsb->m_lua, -1)) {
        long long i = (long long)lua_tonumber(lsb->m_lua, -1);
        if (!(result = pb_write_tag(d, id, 0))) {
            result = pb_write_varint(d, i);
        }
    }
    lua_pop(lsb->m_lua, 1);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int encode_double(lua_sandbox* lsb, output_data* d, char id)
{
    // todo add big endian support if necessary
    double n = lua_tonumber(lsb->m_lua, -1);
    if (pb_write_tag(d, id, 1)) return 1;
    return pb_write_double(d, n);
}

////////////////////////////////////////////////////////////////////////////////
int encode_field_array(lua_sandbox* lsb, output_data* d, int t,
                       const char* representation)
{
    int result = 0, first = 1;
    lua_checkstack(lsb->m_lua, 2);
    lua_pushnil(lsb->m_lua);
    while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
        // numerics are not packed, the space savings aren't worth the extra
        // buffer manipulation
        if (lua_type(lsb->m_lua, -1) != t) {
            snprintf(lsb->m_error_message, ERROR_SIZE, "array has mixed types");
            return 1;
        }
        result = encode_field_value(lsb, d, first, representation);
        first = 0;
        lua_pop(lsb->m_lua, 1); // Remove the value leaving the key on top for
                                // the next interation.
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int encode_field_object(lua_sandbox* lsb, output_data* d)
{
    int result = 0;
    const char* representation = NULL;
    lua_getfield(lsb->m_lua, -1, "representation");
    if (lua_isstring(lsb->m_lua, -1)) {
        representation = lua_tostring(lsb->m_lua, -1);
    }
    lua_getfield(lsb->m_lua, -2, "value");
    result = encode_field_value(lsb, d, 1, representation);
    lua_pop(lsb->m_lua, 2); // remove representation and  value
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int encode_field_value(lua_sandbox* lsb, output_data* d, int first,
                       const char* representation)
{
    int result = 1;
    size_t len;
    const char* s;

    int t = lua_type(lsb->m_lua, -1);
    switch (t) {
    case LUA_TSTRING:
        if (first && representation) { // this uglyness keeps the protobuf
                                       // fields in order without additional
                                       // lookups
            if (pb_write_string(d, 3, representation, strlen(representation))) {
                return 1;
            }
        }
        s = lua_tolstring(lsb->m_lua, -1, &len);
        result = pb_write_string(d, 4, s, len);
        break;
    case LUA_TNUMBER:
        if (first) {
            if (pb_write_tag(d, 2, 0)) return 1;
            if (pb_write_varint(d, 3)) return 1;
            if (representation) {
                if (pb_write_string(d, 3, representation,
                                    strlen(representation))) {
                    return 1;
                }
            }
        }
        result = encode_double(lsb, d, 7);
        break;
    case LUA_TBOOLEAN:
        if (first) {
            if (pb_write_tag(d, 2, 0)) return 1;
            if (pb_write_varint(d, 4)) return 1;
            if (representation) {
                if (pb_write_string(d, 3, representation,
                                    strlen(representation))) {
                    return 1;
                }
            }
        }
        if (pb_write_tag(d, 8, 0)) return 1;
        result = pb_write_bool(d, lua_toboolean(lsb->m_lua, -1));
        break;
    case LUA_TTABLE:
        {
            lua_rawgeti(lsb->m_lua, -1, 1);
            int t = lua_type(lsb->m_lua, -1);
            lua_pop(lsb->m_lua, 1); // remove the array test value
            if (LUA_TNIL == t) {
                result = encode_field_object(lsb, d);
            } else {
                result = encode_field_array(lsb, d, t, representation);
            }
        }
        break;
    default:
        snprintf(lsb->m_error_message, ERROR_SIZE, "unsupported type %d", t);
        result = 1;
    }
    return result;
}

////////////////////////////////////////////////////////////////////////////////
int update_field_length(output_data* d, size_t len_pos)
{
    size_t len = d->m_pos - len_pos - 1;
    if (len < 128) {
        d->m_data[len_pos] = len;
        return 0;
    }
    size_t l = len, cnt = 0;
    while (l) {
        l >>= 7;
        ++cnt;  // compute the number of bytes needed for the varint length
    }
    size_t needed = cnt - 1;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }
    size_t end_pos = d->m_pos + needed;
    memmove(&d->m_data[len_pos + cnt], &d->m_data[len_pos + 1], len);
    d->m_pos = len_pos;
    if (pb_write_varint(d, len)) return 1;
    d->m_pos = end_pos;
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int encode_fields(lua_sandbox* lsb, output_data* d, char id, const char* name)
{
    int result = 0;
    lua_getfield(lsb->m_lua, 1, name);
    if (!lua_istable(lsb->m_lua, -1)) return result;

    size_t len_pos, len;
    lua_checkstack(lsb->m_lua, 2);
    lua_pushnil(lsb->m_lua);
    while (result == 0 && lua_next(lsb->m_lua, -2) != 0) {
        if (pb_write_tag(d, id, 2)) return 1;
        len_pos = d->m_pos;
        if (pb_write_varint(d, 0)) return 1;  // length tbd later
        if (lua_isstring(lsb->m_lua, -2)) {
            const char* s = lua_tolstring(lsb->m_lua, -2, &len);
            if (pb_write_string(d, 1, s, len)) return 1;
        } else {
            snprintf(lsb->m_error_message, ERROR_SIZE,
                     "field name must be a string");
            return 1;
        }
        if (encode_field_value(lsb, d, 1, NULL)) return 1;
        if (update_field_length(d, len_pos)) return 1;
        lua_pop(lsb->m_lua, 1); // Remove the value leaving the key on top for
                                // the next interation.
    }
    lua_pop(lsb->m_lua, 1); // remove the fields table
    return result;
}

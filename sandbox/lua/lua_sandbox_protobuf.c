/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Sandboxed Lua execution @file
#include <stdlib.h>
#include <string.h>
#include "lua_sandbox_protobuf.h"

////////////////////////////////////////////////////////////////////////////////
int pb_write_varint(output_data* d, long long i)
{
    size_t needed = 10;
    if (needed > d->m_size - d->m_pos) {
        if (realloc_output(d, needed)) return 1;
    }

    size_t start = d->m_pos;
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
        if ((result == pb_write_tag(d, id, 0))) {
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
            size_t len;
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

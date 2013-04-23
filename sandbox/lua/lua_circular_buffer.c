/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Lua circular buffer implementation @file
#include <ctype.h>
#include <string.h>
#include <time.h>

#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>

#include "lua_circular_buffer.h"

#define COLUMN_NAME_SIZE 16

static const time_t seconds_in_minute = 60;
static const time_t seconds_in_hour = 60 * 60;
static const time_t seconds_in_day = 60 * 60 * 24;

static const char* column_type_names[] = { "count", "min", "max", "avg",
    "delta", "percentage", NULL };

typedef enum {
    TYPE_COUNT      = 0,
    TYPE_MIN        = 1,
    TYPE_MAX        = 2,
    TYPE_AVG        = 3,
    TYPE_DELTA      = 4,
    TYPE_PERCENTAGE = 5,

    MAX_TYPE
} COLUMN_TYPE;

typedef struct
{
    char        m_name[COLUMN_NAME_SIZE];
    COLUMN_TYPE m_type;
} header_info;

struct circular_buffer
{
    time_t          m_current_time;
    unsigned        m_seconds_per_row;
    unsigned        m_current_row;
    unsigned        m_rows;
    unsigned        m_columns;
    header_info*    m_headers;
    double*         m_values;
    char            m_bytes[1];
};


////////////////////////////////////////////////////////////////////////////////
static void clear_rows(circular_buffer* cb, int num_rows)
{
    if (num_rows >= cb->m_rows) { // clear all
        memset(cb->m_values, 0, sizeof(double) * cb->m_rows * cb->m_columns);
        return;
    }

    unsigned row = cb->m_current_row;
    for (unsigned x = 0; x < num_rows; ++x) {
        ++row;
        if (row >= cb->m_rows) {row = 0;}
        memset(&cb->m_values[row * cb->m_columns], 0,
               sizeof(double) * cb->m_columns);
        // TODO optimize by clearing more than one row at a time
    }
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_new(lua_State* lua)
{
    luaL_argcheck(lua, 3 == lua_gettop(lua), -1,
                  "incorrect number of arguments");
    int rows = luaL_checkint(lua, 1);
    luaL_argcheck(lua, 1 < rows, 1, "rows must be > 1");
    int columns =  luaL_checkint(lua, 2);
    luaL_argcheck(lua, 0 < columns, 2, "columns must be > 0");
    int seconds_per_row = luaL_checkint(lua, 3);
    luaL_argcheck(lua, 0 < seconds_per_row
                  && seconds_per_row <= seconds_in_hour, 3,
                  "seconds_per_row is out of range");

    size_t header_bytes = sizeof(header_info) * columns;
    size_t buffer_bytes = sizeof(double) * rows * columns;
    size_t struct_bytes = sizeof(circular_buffer) - 1; // subtract 1 for the
                                                       // byte already included
                                                       // in the struct

    size_t nbytes = header_bytes + buffer_bytes + struct_bytes;
    circular_buffer* cb = (circular_buffer*)lua_newuserdata(lua, nbytes);
    cb->m_headers = (header_info*)&cb->m_bytes[0];
    cb->m_values = (double*)&cb->m_bytes[header_bytes];

    luaL_getmetatable(lua, heka_circular_buffer);
    lua_setmetatable(lua, -2);

    cb->m_current_time = seconds_per_row * (rows - 1);
    cb->m_current_row = rows - 1;
    cb->m_rows = rows;
    cb->m_columns = columns;
    cb->m_seconds_per_row = seconds_per_row;
    memset(cb->m_bytes, 0, header_bytes + buffer_bytes);
    for (unsigned column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
        snprintf(cb->m_headers[column_idx].m_name, COLUMN_NAME_SIZE,
                 "Column_%d", column_idx+1);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static circular_buffer* check_circular_buffer(lua_State* lua, int min_args)
{
    void* ud = luaL_checkudata(lua, 1, heka_circular_buffer);
    luaL_argcheck(lua, ud != NULL, 1, "invalid userdata type");
    luaL_argcheck(lua, min_args <= lua_gettop(lua), 0,
                  "incorrect number of arguments");
    return (circular_buffer*)ud;
}

///////////////////////////////////////////////////////////////////////////////
static int check_row(lua_State* lua, circular_buffer* cb, int arg, int advance)
{
    time_t t = (time_t)luaL_checknumber(lua, arg) / 1e9;
    t = t - (t % cb->m_seconds_per_row);

    int current_row = cb->m_current_time / cb->m_seconds_per_row;
    int requested_row = t / cb->m_seconds_per_row;
    int row_delta  = requested_row - current_row;
    int row = requested_row % cb->m_rows;

    if (row_delta > 0 && advance) {
        clear_rows(cb, row_delta);
        cb->m_current_time = t;
        cb->m_current_row = row;
    } else if (row_delta <= -(int)cb->m_rows) {
        return -1;
    }
    return row;
}

////////////////////////////////////////////////////////////////////////////////
static int check_column(lua_State* lua, circular_buffer* cb, int arg)
{
    int column = luaL_checkint(lua, arg);
    luaL_argcheck(lua, 1 <= column && column <= cb->m_columns, arg,
                  "column out of range");
    --column; // make zero based
    return column;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_add(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 4);
    int row             = check_row(lua, cb, 2, 1); // advance the buffer
                                                    // forward if necessary
    int column          = check_column(lua, cb, 3);
    double value        = luaL_checknumber(lua, 4);

    if (row != -1) {
        int i = (row * cb->m_columns) + column;
        cb->m_values[i] += value;
        lua_pushinteger(lua, cb->m_values[i]);
    } else {
        lua_pushnil(lua);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_get(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 3);
    int row             = check_row(lua, cb, 2, 0);
    int column          = check_column(lua, cb, 3);

    if (row != -1) {
        lua_pushnumber(lua, cb->m_values[(row * cb->m_columns) + column]);
    } else {
        lua_pushnil(lua);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_set(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 4);
    int row             = check_row(lua, cb, 2, 1); // advance the buffer
                                                    // forward if necessary
    int column          = check_column(lua, cb, 3);
    double value        = luaL_checknumber(lua, 4);

    if (row != -1) {
        cb->m_values[(row * cb->m_columns) + column] = value;
        lua_pushnumber(lua, value);
    } else {
        lua_pushnil(lua);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_set_header(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 4);
    int column          = check_column(lua, cb, 2);
    const char* name    = luaL_checkstring(lua, 3);
    const char* type    = luaL_checkstring(lua, 4);

    strncpy(cb->m_headers[column].m_name, name, COLUMN_NAME_SIZE - 1);
    for (int i = 0; i < MAX_TYPE; ++i) {
        if (strcmp(type, column_type_names[i]) == 0) {
            cb->m_headers[column].m_type = i;

            char* n = cb->m_headers[column].m_name;
            for (int j = 0; n[j] != 0; ++j) {
                if (!isalnum(n[j])) {
                    n[j] = '_';
                }
            }
            break;
        }
    }
    lua_pushinteger(lua, column + 1); // return the 1 based Lua column
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_fromstring(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 2);
    const char* values  = luaL_checkstring(lua, 2);

    int n = 0;
    long long t;
    double value;
    if (!sscanf(values, "%lld %d%n", &t, &cb->m_current_row, &n)) {
        lua_pushstring(lua, "fromstring() invalid time/row");
        lua_error(lua);
        return 0;
    }
    cb->m_current_time = t;
    int offset = n, pos = 0;
    size_t len = cb->m_rows * cb->m_columns;
    while (sscanf(&values[offset], "%lg%n", &value, &n) == 1)
    {
        if (pos == len) {
            lua_pushstring(lua, "fromstring() too many values");
            lua_error(lua);
        }
        offset += n;
        cb->m_values[pos++] = value;
    }
    if (pos != len) {
        lua_pushstring(lua, "fromstring() too few values");
        lua_error(lua);
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int output_circular_buffer(circular_buffer* cb, output_data* output)
{
    // output header
    if (dynamic_snprintf(output, 
                         "{\"time\":%d,\"rows\":%d,\"columns\":%d,\"seconds_per_row\":%d,\"column_info\":[",
                         cb->m_current_time - (cb->m_seconds_per_row * (cb->m_rows - 1)),
                         cb->m_rows,
                         cb->m_columns,
                         cb->m_seconds_per_row)) {
        return 1;
    }

    unsigned column_idx;
    for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
        if (column_idx != 0) {
            if (dynamic_snprintf(output, ",")) return 1;
        }
        if (dynamic_snprintf(output, "{\"name\":\"%s\",\"type\":\"%s\"}", 
                             cb->m_headers[column_idx].m_name,
                             column_type_names[cb->m_headers[column_idx].m_type])) {
            return 1;
        }
    }
    if (dynamic_snprintf(output, "]}\n")) return 1;

    // output buffer data
    unsigned row_idx = cb->m_current_row + 1;
    for (unsigned i = 0; i < cb->m_rows; ++i, ++row_idx) {
        if (row_idx >= cb->m_rows) row_idx = 0;
        for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
            if (column_idx != 0) {
                if (dynamic_snprintf(output, "\t")) return 1;
            }
            if (dynamic_snprintf(output, "%0.9g",
                                 cb->m_values[(row_idx * cb->m_columns) + column_idx])) {
                return 1;
            }
        }
        if (dynamic_snprintf(output, "\n")) return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_circular_buffer(const char* key, circular_buffer* cb, 
                              output_data* output)
{
    output->m_pos = 0;
    if (dynamic_snprintf(output, 
                         "if %s == nil then %s = circular_buffer.new(%d, %d, %d) end\n", 
                         key, 
                         key, 
                         cb->m_rows, 
                         cb->m_columns, 
                         cb->m_seconds_per_row)) {
        return 1;
    }

    unsigned column_idx;
    for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
        if (dynamic_snprintf(output, "%s:set_header(%d, \"%s\", \"%s\")\n",
                             key, 
                             column_idx+1, 
                             cb->m_headers[column_idx].m_name,
                             column_type_names[cb->m_headers[column_idx].m_type])) {
            return 1;
        }
    }

    if (dynamic_snprintf(output, "%s:fromstring(\"%lld %d",
                         key,
                         cb->m_current_time, 
                         cb->m_current_row)) {
        return 1;
    }
    for (unsigned row_idx = 0; row_idx < cb->m_rows; ++row_idx) {
        for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
            if (dynamic_snprintf(output, " %0.9g",
                                 cb->m_values[(row_idx * cb->m_columns) + column_idx])) {
                return 1;
            }
        }
    }
    if (dynamic_snprintf(output, "\")\n")) {
        return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
static const struct luaL_reg circular_bufferlib_f[] =
{
    { "new", circular_buffer_new },
    { NULL, NULL }
};

static const struct luaL_reg circular_bufferlib_m[] =
{
    { "add", circular_buffer_add },
    { "get", circular_buffer_get },
    { "set", circular_buffer_set },
    { "set_header", circular_buffer_set_header },

    { "fromstring", circular_buffer_fromstring }, // used for data restoration
    { NULL, NULL }
};

////////////////////////////////////////////////////////////////////////////////
void luaopen_circular_buffer(lua_State* lua)
{
    luaL_newmetatable(lua, heka_circular_buffer);
    lua_pushliteral(lua, "__index");
    lua_pushvalue(lua, -2);
    lua_rawset(lua, -3);
    luaL_register(lua, NULL, circular_bufferlib_m);
    luaL_register(lua, heka_circular_buffer_table, circular_bufferlib_f);

    // Add an empty metatable to identify core libraries during preservation.
    lua_newtable(lua);
    lua_setmetatable(lua, -2);
    lua_pop(lua, 2); // remove the two circular buffer metatables
}

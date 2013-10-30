/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// @brief Lua circular buffer implementation @file
#include <ctype.h>
#include <float.h>
#include <math.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>

#include "lua_circular_buffer.h"
const char* heka_circular_buffer = "Heka.circular_buffer";
const char* heka_circular_buffer_table = "circular_buffer";

#define COLUMN_NAME_SIZE 16
#define UNIT_LABEL_SIZE 8

static const time_t seconds_in_minute = 60;
static const time_t seconds_in_hour = 60 * 60;
static const time_t seconds_in_day = 60 * 60 * 24;

static const char* column_aggregation_methods[] = { "sum", "min", "max", "avg",
    "none", NULL };
static const char* default_unit = "count";

typedef enum {
    AGGREGATION_SUM     = 0,
    AGGREGATION_MIN     = 1,
    AGGREGATION_MAX     = 2,
    AGGREGATION_AVG     = 3,
    AGGREGATION_NONE    = 4,

    MAX_AGGREGATION
} COLUMN_AGGREGATION;

typedef enum {
    OUTPUT_CBUF     = 0,
    OUTPUT_CBUFD    = 1,

    MAX_OUTPUT_FORMAT
} OUTPUT_FORMAT;

typedef struct
{
    char                m_name[COLUMN_NAME_SIZE];
    char                m_unit[UNIT_LABEL_SIZE];
    COLUMN_AGGREGATION  m_aggregation;
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
    int             m_delta;
    OUTPUT_FORMAT   m_format;
    int             m_ref;
    char            m_bytes[1];
};

////////////////////////////////////////////////////////////////////////////////
static inline time_t get_start_time(circular_buffer* cb)
{
    return cb->m_current_time - (cb->m_seconds_per_row * (cb->m_rows - 1));
}

////////////////////////////////////////////////////////////////////////////////
static void clear_rows(circular_buffer* cb, unsigned num_rows)
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
    int n = lua_gettop(lua);
    luaL_argcheck(lua, n >= 3 && n <= 4, -1,
                  "incorrect number of arguments");
    int rows = luaL_checkint(lua, 1);
    luaL_argcheck(lua, 1 < rows, 1, "rows must be > 1");
    int columns =  luaL_checkint(lua, 2);
    luaL_argcheck(lua, 0 < columns, 2, "columns must be > 0");
    int seconds_per_row = luaL_checkint(lua, 3);
    luaL_argcheck(lua, 0 < seconds_per_row
                  && seconds_per_row <= seconds_in_hour, 3,
                  "seconds_per_row is out of range");
    int delta = 0;
    if (4 == n) delta = lua_toboolean(lua, 4);

    size_t header_bytes = sizeof(header_info) * columns;
    size_t buffer_bytes = sizeof(double) * rows * columns;
    size_t struct_bytes = sizeof(circular_buffer) - 1; // subtract 1 for the
                                                       // byte already included
                                                       // in the struct

    size_t nbytes = header_bytes + buffer_bytes + struct_bytes;
    circular_buffer* cb = (circular_buffer*)lua_newuserdata(lua, nbytes);
    cb->m_ref = LUA_NOREF;
    cb->m_delta = delta;
    cb->m_format = OUTPUT_CBUF;
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
                 "Column_%d", column_idx + 1);
        strncpy(cb->m_headers[column_idx].m_unit, default_unit,
                UNIT_LABEL_SIZE - 1);
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
static int check_row(circular_buffer* cb, double ns, int advance)
{
    time_t t = (time_t)(ns / 1e9);
    t = t - (t % cb->m_seconds_per_row);

    int current_row = cb->m_current_time / cb->m_seconds_per_row;
    int requested_row = t / cb->m_seconds_per_row;
    int row_delta  = requested_row - current_row;
    int row = requested_row % cb->m_rows;

    if (row_delta > 0 && advance) {
        clear_rows(cb, row_delta);
        cb->m_current_time = t;
        cb->m_current_row = row;
    } else if (abs(row_delta) >= (int)cb->m_rows) {
        return -1;
    }
    return row;
}

////////////////////////////////////////////////////////////////////////////////
static int check_column(lua_State* lua, circular_buffer* cb, int arg)
{
    unsigned column = luaL_checkint(lua, arg);
    luaL_argcheck(lua, 1 <= column && column <= cb->m_columns, arg,
                  "column out of range");
    --column; // make zero based
    return column;
}

////////////////////////////////////////////////////////////////////////////////
static void circular_buffer_add_delta(lua_State* lua, circular_buffer* cb, 
                                      double ns, int column, double value)
{
    if (0 == value) return;
    // Storing the deltas in a Lua table allows the sandbox to account for the
    // memory usage. todo: if too inefficient use a C data struct and report 
    // memory usage back to the sandbox
    time_t t = (time_t)(ns / 1e9);
    t = t - (t % cb->m_seconds_per_row);
    lua_getglobal(lua, heka_circular_buffer_table);
    if (lua_istable(lua, -1)) {
        if (cb->m_ref == LUA_NOREF) {
            lua_newtable(lua);
            cb->m_ref = luaL_ref(lua, -2);
        }
        // get the delta table for this cbuf
        lua_rawgeti(lua, -1, cb->m_ref);
        if (!lua_istable(lua, -1)) {
            lua_pop(lua, 2); // remove bogus table and cbuf table
            return;
        }

        // get the delta row using the timestamp
        lua_rawgeti(lua, -1, t);
        if (!lua_istable(lua, -1)) {
            lua_pop(lua, 1); // remove non table entry
            lua_newtable(lua);
            lua_rawseti(lua, -2, t);
            lua_rawgeti(lua, -1, t);
        }

        // get the previous delta value
        lua_rawgeti(lua, -1, column);
        value += lua_tonumber(lua, -1);
        lua_pop(lua, 1); // remove the old value

        // push the new delta
        lua_pushnumber(lua, value);
        lua_rawseti(lua, -2, column);

        lua_pop(lua, 2); // remove ref table, timestamped row
    } else {
        luaL_error(lua, "Could not find table %s", heka_circular_buffer_table);
    }
    lua_pop(lua, 1); // remove the circular buffer table or failed nil
    return; 
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_add(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 4);
    double ns = luaL_checknumber(lua, 2);
    int row             = check_row(cb,
                                    ns,
                                    1); // advance the buffer forward if
                                        // necessary
    int column          = check_column(lua, cb, 3);
    double value        = luaL_checknumber(lua, 4);

    if (row != -1) {
        int i = (row * cb->m_columns) + column;
        cb->m_values[i] += value;
        lua_pushnumber(lua, cb->m_values[i]);
        if (cb->m_delta) {
            circular_buffer_add_delta(lua, cb, ns, column, value);
        }
    } else {
        lua_pushnil(lua);
    }
    return 1; 
}


////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_get(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 3);
    int row             = check_row(cb,
                                    luaL_checknumber(lua, 2),
                                    0);
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
    double ns = luaL_checknumber(lua, 2);
    int row             = check_row(cb,
                                    ns,
                                    1); // advance the buffer forward if
                                        //necessary
    int column          = check_column(lua, cb, 3);
    double value        = luaL_checknumber(lua, 4);

    if (row != -1) {
        int i = (row * cb->m_columns) + column;
        double old = cb->m_values[i];
        cb->m_values[i] = value;
        lua_pushnumber(lua, value);
        if (cb->m_delta) {
            circular_buffer_add_delta(lua, cb, ns, column, value - old);
        }
    } else {
        lua_pushnil(lua);
    }
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_set_header(lua_State* lua)
{
    circular_buffer* cb                 = check_circular_buffer(lua, 3);
    int column                          = check_column(lua, cb, 2);
    const char* name                    = luaL_checkstring(lua, 3);
    const char* unit                    = luaL_optstring(lua, 4, default_unit);
    cb->m_headers[column].m_aggregation = luaL_checkoption(lua, 5, "sum",
                                                           column_aggregation_methods);

    strncpy(cb->m_headers[column].m_name, name, COLUMN_NAME_SIZE - 1);
    char* n = cb->m_headers[column].m_name;
    for (int j = 0; n[j] != 0; ++j) {
        if (!isalnum(n[j])) {
            n[j] = '_';
        }
    }
    strncpy(cb->m_headers[column].m_unit, unit, UNIT_LABEL_SIZE - 1);
    n = cb->m_headers[column].m_unit;
    for (int j = 0; n[j] != 0; ++j) {
        if (n[j] != '/' && n[j] != '*' && !isalnum(n[j])) {
            n[j] = '_';
        }
    }

    lua_pushinteger(lua, column + 1); // return the 1 based Lua column
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static double compute_sum(circular_buffer* cb, unsigned column,
                          unsigned start_row, unsigned end_row)
{
    double result = 0;
    unsigned row = start_row;
    do {
        if (row == cb->m_rows) {
            row = 0;
        }
        result += cb->m_values[(row * cb->m_columns) + column];
    }
    while (row++ != end_row);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
static double compute_avg(circular_buffer* cb, unsigned column,
                          unsigned start_row, unsigned end_row)
{
    double result = 0;
    unsigned row = start_row;
    unsigned row_count = 0;

    do {
        if (row == cb->m_rows) {
            row = 0;
        }
        result += cb->m_values[(row * cb->m_columns) + column];
        ++row_count;
    }
    while (row++ != end_row);
    return result / row_count;
}

// TODO remove - BSD sqrt function using Newton's method
// This is a temporary fix until we figure out why the math sqrt function call
// causes cgo link errors on Windows.
////////////////////////////////////////////////////////////////////////////////
static double bsd_sqrt(double arg)
{
    double x, temp;
    int exp;
    int i;

    if (arg <= 0.) {
        return (0.);
    }
    x = frexp(arg, &exp);
    while (x < 0.5) {
        x *= 2;
        exp--;
    }
    /*
     * NOTE
     * this wont work on 1's comp
     */
    if (exp & 1) {
        x *= 2;
        exp--;
    }
    temp = 0.5 * (1.0 + x);

    while (exp > 60) {
        temp *= (1L << 30);
        exp -= 60;
    }
    while (exp < -60) {
        temp /= (1L << 30);
        exp += 60;
    }
    if (exp >= 0) {
        temp *= 1L << (exp / 2);
    } else {
        temp /= 1L << (-exp / 2);
    }
    for (i = 0; i <= 4; i++) {
        temp = 0.5 * (temp + arg / temp);
    }
    return (temp);
}

////////////////////////////////////////////////////////////////////////////////
static double compute_sd(circular_buffer* cb, unsigned column,
                         unsigned start_row, unsigned end_row)
{
    double avg = compute_avg(cb, column, start_row, end_row);
    double sum_squares = 0;
    double value = 0;
    unsigned row = start_row;
    unsigned row_count = 0;
    do {
        if (row == cb->m_rows) {
            row = 0;
        }
        value = cb->m_values[(row * cb->m_columns) + column] - avg;
        sum_squares += value * value;
        ++row_count;
    }
    while (row++ != end_row);
    return bsd_sqrt(sum_squares / row_count);
}

////////////////////////////////////////////////////////////////////////////////
static double compute_min(circular_buffer* cb, unsigned column,
                          unsigned start_row, unsigned end_row)
{
    double result = DBL_MAX;
    double value = 0;
    unsigned row = start_row;
    do {
        if (row == cb->m_rows) {
            row = 0;
        }
        value = cb->m_values[(row * cb->m_columns) + column];
        if (value < result) {
            result = value;
        }
    }
    while (row++ != end_row);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
static double compute_max(circular_buffer* cb, unsigned column,
                          unsigned start_row, unsigned end_row)
{
    double result = DBL_MIN;
    double value = 0;
    unsigned row = start_row;
    do {
        if (row == cb->m_rows) {
            row = 0;
        }
        value = cb->m_values[(row * cb->m_columns) + column];
        if (value > result) {
            result = value;
        }
    }
    while (row++ != end_row);
    return result;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_compute(lua_State* lua)
{
    static const char* functions[] = { "sum", "avg", "sd", "min", "max", NULL };
    circular_buffer* cb  = check_circular_buffer(lua, 3);
    int function         = luaL_checkoption(lua, 2, NULL, functions);
    int column           = check_column(lua, cb, 3);

    // optional range arguments
    double start_ns = luaL_optnumber(lua, 4, get_start_time(cb) * 1e9);
    double end_ns   = luaL_optnumber(lua, 5, cb->m_current_time * 1e9);
    luaL_argcheck(lua, end_ns >= start_ns, 5, "end must be >= start");

    int start_row = check_row(cb, start_ns, 0);
    int end_row   = check_row(cb, end_ns, 0);
    if (-1 == start_row  || -1 == end_row) {
        lua_pushnil(lua);
        return 1;
    }

    double result = 0;
    switch (function) {
    case 0:
        result = compute_sum(cb, column, start_row, end_row);
        break;
    case 1:
        result = compute_avg(cb, column, start_row, end_row);
        break;
    case 2:
        result = compute_sd(cb, column, start_row, end_row);
        break;
    case 3:
        result = compute_min(cb, column, start_row, end_row);
        break;
    case 4:
        result = compute_max(cb, column, start_row, end_row);
        break;
    }

    lua_pushnumber(lua, result);
    return 1;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_format(lua_State* lua)
{
    static const char* output_types[] = {"cbuf", "cbufd", NULL};
    circular_buffer* cb = check_circular_buffer(lua, 2);
    luaL_argcheck(lua, 2 == lua_gettop(lua), 0,
                  "incorrect number of arguments");

    cb->m_format = luaL_checkoption(lua, 2, NULL, output_types);
    lua_pop(lua, 1); // remove the format
    return 1; // return the circular buffer object
}


////////////////////////////////////////////////////////////////////////////////
const char* get_output_format(circular_buffer* cb)
{
    switch (cb->m_format) {
    case OUTPUT_CBUFD:
       return "cbufd";
    default:
       return "cbuf";
    }
}

////////////////////////////////////////////////////////////////////////////////
static void circular_buffer_delta_fromstring(lua_State* lua, 
                                             circular_buffer* cb,
                                             const char* values,
                                             size_t offset)
{
    int n  = 0;
    double value, ns = 0;
    size_t pos = 0;
    while (sscanf(&values[offset], "%lg%n", &value, &n) == 1) {
        if (pos == 0) { // new row, starts with a time_t
            ns = value * 1e9;
        } else {
            circular_buffer_add_delta(lua, cb, ns, pos-1, value);
        }
        if (pos == cb->m_columns) {
            pos = 0;
        } else {
            ++pos;
        }
        offset += n;
    }
    if (pos != 0) {
        lua_pushstring(lua, "fromstring() invalid delta");
        lua_error(lua);
    }
    return;
}

////////////////////////////////////////////////////////////////////////////////
static int circular_buffer_fromstring(lua_State* lua)
{
    circular_buffer* cb = check_circular_buffer(lua, 2);
    const char* values  = luaL_checkstring(lua, 2);

    int n = 0;
    long long t;
    double value;
    if (!sscanf(values, "%lld %u%n", &t, &cb->m_current_row, &n)) {
        lua_pushstring(lua, "fromstring() invalid time/row");
        lua_error(lua);
    }
    cb->m_current_time = t;
    size_t offset = n, pos = 0;
    size_t len = cb->m_rows * cb->m_columns;
    while (sscanf(&values[offset], "%lg%n", &value, &n) == 1) {
        if (pos == len) {
            if (cb->m_delta) {
                circular_buffer_delta_fromstring(lua, cb, values, offset);
                return 0;
            } else {
                lua_pushstring(lua, "fromstring() too many values");
                lua_error(lua);
            }
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
int output_circular_buffer_full(circular_buffer* cb, output_data* output)
{
    unsigned column_idx;
    unsigned row_idx = cb->m_current_row + 1;
    for (unsigned i = 0; i < cb->m_rows; ++i, ++row_idx) {
        if (row_idx >= cb->m_rows) row_idx = 0;
        for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
            if (column_idx != 0) {
                if (dynamic_snprintf(output, "\t")) return 1;
            }
            if (serialize_double(output,
                                 cb->m_values[(row_idx * cb->m_columns) + column_idx])) {
                return 1;
            }
        }
        if (dynamic_snprintf(output, "\n")) return 1;
    }
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int output_circular_buffer_cbufd(lua_State *lua, circular_buffer* cb,
                                 output_data* output)
{
    lua_getglobal(lua, heka_circular_buffer_table);
    if (lua_istable(lua, -1)) {
        // get the delta table for this cbuf
        lua_rawgeti(lua, -1, cb->m_ref);
        if (!lua_istable(lua, -1)) {
            lua_pop(lua, 2); // remove bogus table and cbuf table
            luaL_error(lua, "Could not find the delta table");
        }
        lua_pushnil(lua);
        while (lua_next(lua, -2) != 0) {
            if (!lua_istable(lua, -1)) {
                luaL_error(lua, "Invalid delta table structure");
            }
            if (serialize_double(output, lua_tonumber(lua, -2))) return 1; 
            for (unsigned column_idx = 0; column_idx < cb->m_columns;
                 ++column_idx) {
                if (dynamic_snprintf(output, "\t")) return 1;
                lua_rawgeti(lua, -1, column_idx);
                if (serialize_double(output, lua_tonumber(lua, -1))) return 1;
                lua_pop(lua, 1); // remove the number
            }
            if (dynamic_snprintf(output, "\n")) return 1;
            lua_pop(lua, 1); // remove the value, keep the key
        }
        lua_pop(lua, 1); // remove the delta table

        // delete the delta table
        lua_pushnil(lua);
        lua_rawseti(lua, -2, cb->m_ref);
        cb->m_ref = LUA_NOREF;
    } else {
        luaL_error(lua, "Could not find table %s", heka_circular_buffer_table);
    }
    lua_pop(lua, 1); // remove the circular buffer table or failed nil
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int output_circular_buffer(lua_State *lua, circular_buffer* cb, 
                           output_data* output)
{
    if (OUTPUT_CBUFD == cb->m_format) {
        if (cb->m_ref == LUA_NOREF) return 0;
    }
    if (dynamic_snprintf(output,
                         "{\"time\":%lld,\"rows\":%d,\"columns\":%d,\"seconds_per_row\":%d,\"column_info\":[",
                         (long long)get_start_time(cb),
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
        if (dynamic_snprintf(output, "{\"name\":\"%s\",\"unit\":\"%s\",\"aggregation\":\"%s\"}",
                             cb->m_headers[column_idx].m_name,
                             cb->m_headers[column_idx].m_unit,
                             column_aggregation_methods[cb->m_headers[column_idx].m_aggregation])) {
            return 1;
        }
    }
    if (dynamic_snprintf(output, "]}\n")) return 1;

    if (OUTPUT_CBUFD == cb->m_format) {
        return output_circular_buffer_cbufd(lua, cb, output);
    }
    return output_circular_buffer_full(cb, output);
}

////////////////////////////////////////////////////////////////////////////////
int serialize_circular_buffer_delta(lua_State *lua, circular_buffer* cb,
                                 output_data* output)
{
    if (cb->m_ref == LUA_NOREF) return 0;
    lua_getglobal(lua, heka_circular_buffer_table);
    if (lua_istable(lua, -1)) {
        // get the delta table for this cbuf
        lua_rawgeti(lua, -1, cb->m_ref);
        if (!lua_istable(lua, -1)) {
            lua_pop(lua, 2); // remove bogus table and cbuf table
            luaL_error(lua, "Could not find the delta table");
        }
        lua_pushnil(lua);
        while (lua_next(lua, -2) != 0) {
            if (!lua_istable(lua, -1))  {
                luaL_error(lua, "Invalid delta table structure");
            }
            if (dynamic_snprintf(output, " ")) return 1;
            if (serialize_double(output, lua_tonumber(lua, -2))) return 1; 
            for (unsigned column_idx = 0; column_idx < cb->m_columns;
                 ++column_idx) {
                if (dynamic_snprintf(output, " ")) return 1;
                lua_rawgeti(lua, -1, column_idx);
                if (serialize_double(output, lua_tonumber(lua, -1))) return 1;
                lua_pop(lua, 1); // remove the number
            }
            lua_pop(lua, 1); // remove the value, keep the key
        }
        lua_pop(lua, 1); // remove the delta table

        // delete the delta table
        lua_pushnil(lua);
        lua_rawseti(lua, -2, cb->m_ref);
        cb->m_ref = LUA_NOREF;
    } else {
        luaL_error(lua, "Could not find table %s", heka_circular_buffer_table);
    }
    lua_pop(lua, 1); // remove the circular buffer table or failed nil
    return 0;
}

////////////////////////////////////////////////////////////////////////////////
int serialize_circular_buffer(lua_State *lua, const char* key, 
                              circular_buffer* cb, output_data* output)
{
    output->m_pos = 0;
    char* delta = "";
    if (cb->m_delta) {
        delta = ", true";
    }
    if (dynamic_snprintf(output,
                         "if %s == nil then %s = circular_buffer.new(%d, %d, %d%s) end\n",
                         key,
                         key,
                         cb->m_rows,
                         cb->m_columns,
                         cb->m_seconds_per_row,
                         delta)) {
        return 1;
    }
    
    unsigned column_idx;
    for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
        if (dynamic_snprintf(output, "%s:set_header(%d, \"%s\", \"%s\", \"%s\")\n",
                             key,
                             column_idx + 1,
                             cb->m_headers[column_idx].m_name,
                             cb->m_headers[column_idx].m_unit,
                             column_aggregation_methods[cb->m_headers[column_idx].m_aggregation])) {
            return 1;
        }
    }

    if (dynamic_snprintf(output, "%s:fromstring(\"%lld %d",
                         key,
                         (long long)cb->m_current_time,
                         cb->m_current_row)) {
        return 1;
    }
    for (unsigned row_idx = 0; row_idx < cb->m_rows; ++row_idx) {
        for (column_idx = 0; column_idx < cb->m_columns; ++column_idx) {
            if (dynamic_snprintf(output, " ")) return 1;
            if (serialize_double(output,
                                 cb->m_values[(row_idx * cb->m_columns) + column_idx])) {
                return 1;
            }
        }
    }
    if (serialize_circular_buffer_delta(lua, cb, output)) {
        return 1;
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
    { "compute", circular_buffer_compute },
    { "format", circular_buffer_format },

    { "fromstring", circular_buffer_fromstring }, // used for data restoration
    { NULL, NULL }
};

////////////////////////////////////////////////////////////////////////////////
int luaopen_circular_buffer(lua_State* lua)
{
    luaL_newmetatable(lua, heka_circular_buffer);
    lua_pushvalue(lua, -1);
    lua_setfield(lua, -2, "__index");
    luaL_register(lua, NULL, circular_bufferlib_m);
    luaL_register(lua, heka_circular_buffer_table, circular_bufferlib_f);
    return 1;
}

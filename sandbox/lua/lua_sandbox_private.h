/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Lua lua_sandbox for Heka plugins @file
#ifndef lua_sandbox_private_h_
#define lua_sandbox_private_h_

#include <lua.h>
#include "lua_sandbox.h"

#define ERROR_SIZE 255
#define OUTPUT_SIZE 1024
#define MAX_MEMORY 1024 * 1024 * 8
#define MAX_INSTRUCTION 1000000
#define MAX_OUTPUT 1024 * 63

typedef struct
{
    size_t m_size;
    size_t m_pos;
    char*  m_data;
} output_data;

struct lua_sandbox
{
    lua_State*      m_lua;
    void*           m_go;
    sandbox_status  m_status;
    output_data     m_output;
    char*           m_lua_file;
    unsigned        m_usage[MAX_USAGE_TYPE][MAX_USAGE_STAT];
    char            m_error_message[ERROR_SIZE];
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
/// Sandbox management functions.
////////////////////////////////////////////////////////////////////////////////
/** 
 * Performs the library load and secures the sandbox environment for use.
 * 
 * @param lua Pointer to the Lua state.
 * @param table Name of the table being loaded.
 * @param f Pointer to the table load function.
 * @param disable Array of function names to disable in the loaded table.
 */
void load_library(lua_State* lua, const char* table, lua_CFunction f,
                  const char** disable);

/** 
 * Implementation of the memory allocator for the Lua state.
 *  
 * See: http://www.lua.org/manual/5.1/manual.html#lua_Alloc 
 * 
 * @param ud Pointer to the lua_sandbox
 * @param ptr Pointer to the memory block being allocated/reallocated/freed.
 * @param osize The original size of the memory block.
 * @param nsize The new size of the memory block.
 * 
 * @return void* A pointer to the memory block.
 */
void* memory_manager(void* ud, void* ptr, size_t osize, size_t nsize);

/** 
 * Lua hook to monitor the instruction usage of the sandbox.
 * 
 * @param lua Pointer to the Lua state.
 * @param ar Pointer to the Lua debug interface.
 */
void instruction_manager(lua_State* lua, lua_Debug* ar);

/** 
 * Extracts the current instruction usage count from the Lua state. 
 * 
 * @param lsb Pointer to the sandbox.
 * 
 * @return size_t The number of instructions used in the last function call
 */
size_t instruction_usage(lua_sandbox* lsb);

/** 
 * Tears down the sandbox on error.
 * 
 * @param lsb Pointer to the sandbox.
 */
void sandbox_terminate(lua_sandbox* lsb);

////////////////////////////////////////////////////////////////////////////////
/// Sandbox global data preservation functions.
////////////////////////////////////////////////////////////////////////////////
/** 
 * Serialize all user global data to disk.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data_file Filename where the data will be written (create/overwrite)
 * 
 * @return int Zero on success, non-zero on failure.
 */
int preserve_global_data(lua_sandbox* lsb, const char* data_file);

/** 
 * Serializes a Lua table structure.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data Pointer to the serialization state data.
 * @param parent Index pointing to the parent's name in the table array.
 * 
 * @return int Zero on success, non-zero on failure.
 */
int serialize_table(lua_sandbox* lsb, serialization_data* data, size_t parent);

/** 
 * Serializes a Lua table structure as JSON.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data Pointer to the serialization state data.
 * @param isHash True if this table is a hash, false if it is an array.
 * 
 * @return int Zero on success, non-zero on failure.
 */
int serialize_table_as_json(lua_sandbox* lsb,
                            serialization_data* data,
                            int isHash);

/** 
 * Serializes a Lua data value.
 * 
 * @param lsb Pointer to the sandbox.
 * @param index Lua stack index where the data resides.
 * @param output Pointer the output collector.
 * 
 * @return int
 */
int serialize_data(lua_sandbox* lsb, int index, output_data* output);

/** 
 * Serializes a Lua data as JSON.
 * 
 * @param lsb Pointer to the sandbox.
 * @param index Lua stack index where the data resides.
 * @param output Pointer the output collector.
 * 
 * @return int
 */
int serialize_data_as_json(lua_sandbox* lsb, int index, output_data* output);

/** 
 * Determines the name of the userdata type
 * 
 * @param lua Lua State
 * @param ud Userdata pointer
 * @param index Index on the stack where the userdata pointer resides
 * 
 * @return const char* NULL if not found
 */
const char* userdata_type(lua_State* lua, void* ud, int index);

/** 
 * Serializes a table key value pair.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data Pointer to the serialization state data.
 * @param parent Index pointing to the parent's name in the table array.
 * 
 * @return int Zero on success, non-zero on failure.
 */
int serialize_kvp(lua_sandbox* lsb, serialization_data* data, size_t parent);

/**
 * Checks to see a string key starts with a '_' in which case it will be 
 * ignored. 
 * 
 * @param lsb Pointer to the sandbox.
 * @param index Lua stack index where the key resides.
 * 
 * @return int True if the key should not be serialized.
 */
int ignore_key(lua_sandbox* lsb, int index);

/** 
 * Serializes a table key value pair as JSON.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data Pointer to the serialization state data.
 * @param isHash True if this kvp is part of a hash, false if it is in an array.
 *  
 * @return int Zero on success, non-zero on failure.
 */
int serialize_kvp_as_json(lua_sandbox* lsb,
                          serialization_data* data,
                          int isHash);

/** 
 * Looks for a table to see if it has already been processed. 
 * 
 * @param tra Pointer to the table references.
 * @param ptr Pointer value of the table.
 * 
 * @return table_ref* NULL if not found.
 */
table_ref* find_table_ref(table_ref_array* tra, const void* ptr);

/** 
 * Adds a table to the processed array.
 * 
 * @param tra Pointer to the table references.
 * @param ptr Pointer value of the table.
 * @param name_pos Index pointing to name in the table array.
 * 
 * @return table_ref* Pointer to the table reference or NULL if out of memory.
 */
table_ref* add_table_ref(table_ref_array* tra, const void* ptr,
                         size_t name_pos);

/** 
 * Helper function to determine what data should not be serialized.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data Pointer to the serialization state data.  
 * @param index Lua stack index where the data resides.
 * 
 * @return int
 */
int ignore_value_type(lua_sandbox* lsb, serialization_data* data, int index);

/** 
 * Helper function to determine what data should not be serialized to JSON.
 * 
 * @param lsb Pointer to the sandbox.
 * @param index Lua stack index where the data resides.
 * 
 * @return int
 */
int ignore_value_type_json(lua_sandbox* lsb, int index);

/** 
 * Restores previously serialized data from disk.
 * 
 * @param lsb Pointer to the sandbox.
 * @param data_file Filename from where the data will be read.
 * 
 * @return int Zero on success, non-zero on failure.
 */
int restore_global_data(lua_sandbox* lsb, const char* data_file);

/** 
 * In memory output stream.
 * 
 * @param output Pointer the output collector.
 * @param fmt Printf format specifier.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int dynamic_snprintf(output_data* output, const char* fmt, ...);

////////////////////////////////////////////////////////////////////////////////
/// Lua to C function interface
////////////////////////////////////////////////////////////////////////////////
/** 
 * Collect sandbox output into an in memory buffer.
 * 
 * @param lua Pointer to the Lua state.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int output(lua_State* lua);

////////////////////////////////////////////////////////////////////////////////
/// Lua to Go function interface
////////////////////////////////////////////////////////////////////////////////
/** 
 * Reads a data field from a Heka message and returns the value.
 * 
 * @param lua Pointer to the Lua state.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int read_message(lua_State* lua);

/** 
 * Inject a message into Heka using the output buffer's contents as the message 
 * payload. 
 * 
 * @param lua Pointer to the Lua state.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int inject_message(lua_State* lua);

#endif

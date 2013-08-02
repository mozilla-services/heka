/* -*- Mode: C; tab-width: 8; indent-tabs-mode: nil; c-basic-offset: 2 -*- */
/* vim: set ts=2 et sw=2 tw=80: */
/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

/// Lua lua_sandbox protobuf encoding for Heka plugins @file
#ifndef lua_sandbox_protobuf_h_
#define lua_sandbox_protobuf_h_
#include "lua_sandbox_private.h"

/** 
 * Writes a varint encoded number to the output buffer.
 * 
 * @param d Pointer to the output data buffer.
 * @param i Number to be encoded.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int pb_write_varint(output_data* d, long long i);

/** 
 * Writes a double to the output buffer.
 * 
 * @param d Pointer to the output data buffer.
 * @param i Double to be encoded.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int pb_write_double(output_data* d, double i);

/** 
 * Writes a bool to the output buffer.
 * 
 * @param d Pointer to the output data buffer.
 * @param i Number to be encoded.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int pb_write_bool(output_data* d, int i);

/** 
 * Writes a field tag (tag id/wire type) to the output buffer.
 * 
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * @param wire_type Field wire type.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int pb_write_tag(output_data* d, char id, char wire_type);

/** 
 * Writes a string to the output buffer.
 * 
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * @param s  String to output.
 * @param len Length of s.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int pb_write_string(output_data* d, char id, const char* s, size_t len);

/** 
 * Retrieve the string value for a Lua table entry (the table should be on top 
 * of the stack).  If the entry is not found or not a string nothing is encoded.
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * @param name Key used for the Lua table entry lookup.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_string(lua_sandbox* lsb, output_data* d, char id, const char* name);

/** 
 * Retrieve the numeric value for a Lua table entry (the table should be on top 
 * of the stack).  If the entry is not found or not a number nothing is encoded, 
 * otherwise the number is varint encoded. 
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * @param name Key used for the Lua table entry lookup.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_int(lua_sandbox* lsb, output_data* d, char id, const char* name);

/** 
 * Encodes the entry on top of the Lua stack as a double. If the entry is not a 
 * number nothing is encoded. 
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_double(lua_sandbox* lsb, output_data* d, char id);

/** 
 * Encodes a field that has an array of values. 
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param ltype Lua type of the array values.
 * @param representation String representation of the field i.e., "ms"
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_field_array(lua_sandbox* lsb, output_data* d, int ltype,
                       const char* representation);

/** 
 * Encodes a field that contains metadata in addition to its value.
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_field_object(lua_sandbox* lsb, output_data* d);

/** 
 * Encodes the field value.
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param first Boolean indicator used to add addition protobuf data in the 
 *              correct order.
 * @param representation String representation of the field i.e., "ms"
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_field_value(lua_sandbox* lsb, output_data* d, int first,
                       const char* representation);

/** 
 * Updates the field length in the output buffer once the size is known, this 
 * allows for single pass encoding.
 * 
 * @param d  Pointer to the output data buffer.
 * @param len_pos Position in the output buffer where the length should be 
 *                written.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int update_field_length(output_data* d, size_t len_pos);

/** 
 * Iterates over the specified Lua table encoding the contents as user defined 
 * message fields. 
 * 
 * @param lsb  Pointer to the sandbox.
 * @param d  Pointer to the output data buffer.
 * @param id Field identifier.
 * @param name Key used for the Lua table entry lookup.
 * 
 * @return int Zero on success, non-zero if out of memory.
 */
int encode_fields(lua_sandbox* lsb, output_data* d, char id, const char* name);

#endif

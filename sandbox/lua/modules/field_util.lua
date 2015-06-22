-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Module contains utility functions for setting up fields
for various purposes.

API
^^^

**field_interp(field)**

**field_map(fields_str)**
    Returns a table of fields that match the space delimited
    input string of fields.  This can be used to provide input to
    other functions such as a list of fields to skip or use for tags.

    *Arguments*
        - fields_str - space delimited list of fields. If this is empty
        all base fields will be returned.

    *Return*
        Table with the fields found in the space delimited input string,
        boolean indicating all base fields are to be used, boolean
        indicating all fields are to be used.

**message_timestamp(timestamp_precision)**

**timestamp_divisor(timestamp_precision)**
    Returns a number that can be used to divide the default heka
    timestamp (in nanoseconds) to a smaller precision.

    *Arguments*
        - timestamp_precision - string of "s", "m", "h"
    *Return*
        Number value that is to be used as a divisor for the Timestamp

**used_base_fields(skip_fields)**
    Returns a table of base fields that are not found in the input table.
    This is useful to provide a lookup table that is used to decide
    whether or not a field should be included in an output by performing
    a simple lookup against it.

    *Arguments*
        - skip_fields - Table of fields to be skipped from use.

    *Return*
    A table of base fields that are not found in the input table.

--]]

local math = require "math"
local pairs = pairs
local read_message = read_message
local require = require
local string = require "string"

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

base_fields_list = {
    Type = true,
    Payload = true,
    Hostname = true,
    Pid = true,
    Logger = true,
    Severity = true,
    EnvVersion = true
}

base_fields_tag_list = {
    Type = true,
    Hostname = true,
    Severity = true,
    Logger = true
}

local function timestamp_divisor(timestamp_precision)
    -- Default is to divide ns to ms
    local timestamp_divisor = 1e6
    -- Divide ns to s
    if timestamp_precision == "s" then
        timestamp_divisor = 1e9
    -- Divide ns to m
    elseif timestamp_precision == "m" then
        timestamp_divisor = 1e9 * 60
    -- Divide ns to h
    elseif timestamp_precision == "h" then
        timestamp_divisor = 1e9 * 60 * 60
    end
    return timestamp_divisor
end

--[[ Public Interface --]]

function field_interp(field)
    local interp = require "msg_interpolate"
    local interp_field
    -- If the %{field} substitutions are defined in the field,
    -- replace them with the actual values from the message here
    if string.find(field, "%%{[%w%p]-}") then
        interp_field = interp.interpolate_from_msg(field, nil)
    else
        interp_field = field
    end
    return interp_field
end

function field_map(fields_str)
    local fields = {}
    local all_base_fields = false
    local all_fields = false

    if fields_str then
        for field in string.gmatch(fields_str, "[%S]+") do
            fields[field] = true
            if field == "**all_base**" then
                all_base_fields = true
            end
            if field == "**all**" then
                all_fields = true
            end
        end

        for field in pairs(base_fields_list) do
            if all_base_fields or all_fields then
                fields[field] = true
            end
        end
    else
        fields = base_fields_list
        all_base_fields = true
    end

    return fields, all_base_fields, all_fields
end

function message_timestamp(timestamp_precision)
    local message_timestamp = read_message("Timestamp")
    message_timestamp = math.floor(message_timestamp / timestamp_divisor(timestamp_precision))
    return message_timestamp
end

function used_base_fields(skip_fields)
    local fields = {}
    for field in pairs(base_fields_list) do
        if not skip_fields[field] then
            fields[field] = true
        end
    end
    return fields
end

return M


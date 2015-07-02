-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Provides functions to convert full Heka message contents to line protocol for
InfluxDB HTTP write API (new in InfluxDB v0.9.0) or Graphite/Carbon.
Optionally includes all standard message fields as tags or fields and
iterates through all of the dynamic fields
to add as points (series), skipping any fields explicitly omitted using the
`skip_fields` config option.  It can also map any Heka message fields as tags
in the request sent to the InfluxDB write API, using the `tag_fields` config
option.  All dynamic fields in the Heka message are converted to separate points
separated by newlines that are submitted to InfluxDB.

API
^^^

**carbon_line_msg()**
    Wrapper function that calls others within this module and the field_util
    module to generate a table of Carbon line protocol messages that are
    derived from the dynamic fields in a Heka message. Base fields or
    dynamic fields can be used in the metric name portion of the message.
    Configuration is implemented in the encoders that utilize this module.
    This function takes no arguments.
    
**influxdb_line_msg()**
    Wrapper function that calls others within this module and the field_util
    module to generate a table of InfluxDB line protocol messages that are
    derived from the base or dynamic fields in a Heka message. Base fields or
    dynamic fields can be used in the metric name portion of the message and 
    included as tags if desired.  Configuration is implemented in the encoders
    that utilize this module.  This function takes no arguments.

--]=]

local field_util = require "field_util"
local math = require "math"
local read_config = read_config
local read_message = read_message
local read_next_field = read_next_field
local pairs = pairs
local string = require "string"
local table = require "table"
local tostring = tostring
local type = type

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

if not timestamp_precision then
    timestamp_precision = "ms"
end
if not decimal_precision then
    decimal_precision = "6"
end
if not name_prefix then
    name_prefix = ""
end
if not name_prefix_delimiter then
    name_prefix_delimiter = ""
end
if not value_field_name then
    value_field_name = "value"
end

-- Remove blacklisted fields from the set of base fields that we use, and
-- create a table of dynamic fields to skip.
local skip_fields_str = read_config("skip_fields") or ""
local skip_fields, skip_fields_all_base = field_util.field_map(skip_fields_str)
local used_base_fields = field_util.used_base_fields(skip_fields)

-- Create and populate a table of fields to be used as tags
local tag_fields_str = read_config("tag_fields") or "**all_base**"
local used_tag_fields, tag_fields_all_base, tag_fields_all = field_util.field_map(tag_fields_str)

local function tags_table()
    -- Initialize and populate the table of tags to include in the
    -- InfluxDB write API message
    -- Convert value to a string as this is required by the API
    local message_tags = {}
    for field in pairs(field_util.base_fields_list) do
        local base_field_value = read_message(field)
        field = field:gsub("([ ,])", "\\%1")
        if (tag_fields_all or tag_fields_all_base)
            and field_util.base_fields_tag_list[field] and type(base_field_value) ~= "number" then
            table.insert(message_tags, field.."="..tostring(base_field_value))
        elseif used_tag_fields[field] and field_util.base_fields_tag_list[field]
            and type(base_field_value) ~= "number" then
            table.insert(message_tags, field.."="..tostring(base_field_value))
        end
    end
    return message_tags
end

local function points_tags_tables()
    -- Initialize the table of data points and populate it with data
    -- from the Heka message.  When skip_fields includes "**all_base**",
    -- Only dynamic fields are included as InfluxDB data points, while
    -- the base fields serve as tags for them. If skip_fields does not
    -- define any base fields, they are added to the fields of each data
    -- point and each dynamic field value is set as the "value" field.
    -- Adding "**all_base**" is recommended to avoid redundant data being
    -- stored in each data point (the base fields as fields and as tags).
    local points = {}
    local tags
    if not carbon_format then
        tags = tags_table()
    end

    while true do
        -- Iterate through Fields array in the message
        local typ, name, value = read_next_field()
        -- Exit the perpetual loop when the iteration is complete
        if not typ then break end

        -- Force to remain as a true float if value is 0; otherwise
        -- InfluxDB will reject the point if previous values were
        -- actual floats and Carbon will reject it altogether
        if value == 0 and typ == 3 then
            value = "0.0"
        end

        -- Replace non-word characters with underscores
        if carbon_format then
            name = name:gsub("[^%w]", "_")
        end

        -- Include the dynamic fields as tags if they are defined in
        -- configuration or the magic value "**all**" is defined.
        -- Convert value to a string as this is required by the API
        if not carbon_format and (tag_fields_all or used_tag_fields[name])
            and type(value) ~= "number" then
            table.insert(tags, name:gsub("([ ,])", "\\%1").."="..tostring(value:gsub("([ ,])", "\\%1")))
        end

        -- Only process fields that are not requested to be skipped
        if not skip_fields_str or not skip_fields[name] then
            -- Set the name attribute of this table by concatenating name_prefix
            -- with the name of this particular field
            points[field_util.field_interp(name_prefix)..name_prefix_delimiter..name] = value
        end
    end
    if carbon_format then
        return points
    else
        return points, tags
    end
end

--[[ Public Interface --]]

function carbon_line_msg()
    local api_message = {}
    local message_timestamp = field_util.message_timestamp(timestamp_precision)
    local points = points_tags_tables()
    for name, value in pairs(points) do
        if type(value) == "number" or value == "0.0" or value == "0" then
            table.insert(api_message, name.." "..value.." "..message_timestamp)
        end
    end
    return api_message
end

function influxdb_line_msg()
    local api_message = {}
    local message_timestamp = field_util.message_timestamp(timestamp_precision)
    local points, tags = points_tags_tables()
    -- Build a table of data points that we will eventually convert
    -- to a newline delimited list of InfluxDB write API line protocol
    -- formatted values that are then injected back into the pipeline
    for name, value in pairs(points) do
        -- Wrap in double quotes and escape embedded double quotes
        -- as defined by the protocol
        if type(value) == "string" then
            value = '"'..value:gsub('"', '\\"')..'"'
        end

        -- Escape spaces and commas as defined by the protocol
        name = name:gsub("([ ,])", "\\%1")

        -- Always send numbers as formatted floats, so InfluxDB will accept
        -- them if they happen to change from ints to floats between
        -- values.  Forcing them to always be floats avoids this.
        -- Use the decimal_precision config option to control the
        -- numbers after the decimal that are printed.
        if type(value) == "number" then
            value = string.format("%."..decimal_precision.."f", value)
        end
        -- Format the line differently based on the presence of tags
        -- i.e. length of the tags table is > 0
        if tags and #tags > 0 then
            table.insert(api_message, name..","..table.concat(tags, ",").." ".."value="..value.." "..message_timestamp)
        else
            table.insert(api_message, name.." "..value_field_name.."="..value.." "..message_timestamp)
        end
    end
    return api_message
end

return M


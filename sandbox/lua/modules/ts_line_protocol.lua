-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Provides functions to convert full Heka message contents to line protocol for
InfluxDB HTTP write API (new in InfluxDB v0.9.0) or Graphite/Carbon and
any other time series data store that this functionality makes sense for.
Optionally includes all standard message fields as tags or fields and
iterates through all of the dynamic fields to add as measurements, skipping
any fields explicitly omitted using the `skip_fields` config option.  It can
also map any Heka message fields as tags in the request sent to the InfluxDB
write API, using the `tag_fields` config option.  All dynamic fields in the
Heka message are converted to separate points separated by newlines that are
submitted to InfluxDB, unless source_value_field is defined.

API
^^^

**carbon_line_msg(config)**
    Wrapper function that calls others within this module and the field_util
    module to generate a table of Carbon line protocol messages that are
    derived from the dynamic fields in a Heka message. Base fields or
    dynamic fields can be used in the metric name portion of the message.
    Configuration is implemented in the encoders that utilize this module.

    *Arguments*
        - config (table or nil)
            Table of config option overrides that come from the client
            of this API.  Defaults for this module are defined within
            the set_config function and clients implementing this API
            can reference it for available options.

    *Return*
        A table of Carbon line protocol messages ready to be sent to
        a Carbon server after being looped through in an encoder implementing
        this API.

**influxdb_line_msg(config)**
    Wrapper function that calls others within this module and the field_util
    module to generate a table of InfluxDB line protocol messages that are
    derived from the base or dynamic fields in a Heka message. Base fields or
    dynamic fields can be used in the metric name portion of the message and
    included as tags if desired.  Configuration is implemented in the encoders
    that utilize this module.

    *Arguments*
        - config (table or nil)
            Table of config option overrides that come from the client
            of this API.  Defaults for this module are defined within
            the set_config function and clients implementing this API
            can reference it for available options.

    *Return*
        A table of InfluxDB line protocol messages ready to be sent to
        an InfluxDB server after being looped through in an encoder
        implementing this API.

**set_config(client_config)**
    This function takes a table of configuration options as input that can
    override the defaults that are set within it.  The table is then used to
    compare with the default `module_config`, updates that table and then
    returns it to the client calling this API. Calls are then made out to
    public functions exposed by the field_util module to populate the tables
    and variables related to base fields, tagging fields and skipping fields
    kept within the module_config table. Clients utilizing this API
    must call this function first to set the configuration for subsequent
    calls to public functions that it exposes.

    *Arguments*
        - client_config (table or nil)
            Table of configuration option overrides with the keys being
            the option names and the values being the option values.

    *Return*
        A table of configuration options that can then be passed to other
        public functions that this API exposes when calling them.

--]=]

local decode_message = decode_message
local interp = require "msg_interpolate"
local ipairs = ipairs
local field_util = require "field_util"
local math = require "math"
local read_config = read_config
local read_message = read_message
local pairs = pairs
local string = require "string"
local table = require "table"
local tostring = tostring
local type = type

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

local function influxdb_kv_fmt(string)
    return tostring(string):gsub("([ ,])", "\\%1")
end

local function points_tags_tables(config)
    local name_prefix = config.name_prefix or ""
    if config.interp_name_prefix then
        name_prefix = interp.interpolate_from_msg(name_prefix)
    end
    local name_prefix_delimiter = config.name_prefix_delimiter or ""
    local used_tag_fields = config.used_tag_fields
    local skip_fields = config.skip_fields

    -- Initialize the tags table, including base field tag values in
    -- list if the magic **all** or **all_base** config values are
    -- defined.
    local tags = {}
    if not config.carbon_format
        and (config.tag_fields_all
        or config.tag_fields_all_base
        or config.used_tag_fields) then
            for field in pairs(field_util.base_fields_tag_list) do
                if config.tag_fields_all or config.tag_fields_all_base or used_tag_fields[field] then
                    local value = read_message(field)
                    table.insert(tags, influxdb_kv_fmt(field).."="..tostring(influxdb_kv_fmt(value)))
                end
            end
    end

    -- Initialize the table of data points and populate it with data
    -- from the Heka message.  When skip_fields includes "**all_base**",
    -- only dynamic fields are included as InfluxDB data points, while
    -- the base fields serve as tags for them. If skip_fields does not
    -- define any base fields, they are added to the fields of each data
    -- point and each dynamic field value is set as the "value" field.
    -- Setting skip_fields to "**all_base**" is recommended to avoid
    -- redundant data being stored in each data point (the base fields
    -- as fields and as tags).
    local points = {}
    local msg = decode_message(read_message("raw"))
    if msg.Fields then
        for _, field_entry in ipairs(msg.Fields) do
            local field = field_entry["name"]
            local value
            for _, field_value in ipairs(field_entry["value"]) do
                value = field_value
            end

            -- Replace non-word characters with underscores for Carbon
            -- to avoid periods resulting in extraneous directories
            if config["carbon_format"] then
                field = field:gsub("[^%w]", "_")
            end

            -- Include the dynamic fields as tags if they are defined in
            -- configuration or the magic value "**all**" is defined.
            -- Convert value to a string as this is required by the API
            if not config["carbon_format"]
                and (config["tag_fields_all"]
                or (config["used_tag_fields"] and used_tag_fields[field])) then
                    table.insert(tags, influxdb_kv_fmt(field).."="..tostring(influxdb_kv_fmt(value)))
            end

            if config["source_value_field"] and field == config["source_value_field"] then
                points[name_prefix] = value
            -- Only add fields that are not requested to be skipped
            elseif not config["skip_fields_str"]
                or (config["skip_fields"] and not skip_fields[field]) then
                    -- Set the name attribute of this table by concatenating name_prefix
                    -- with the name of this particular field
                    points[name_prefix..name_prefix_delimiter..field] = value
            end
        end
    else
        return 0
    end

    return points, tags
end

--[[ Public Interface]]

function carbon_line_msg(config)
    local api_message = {}
    local message_timestamp = field_util.message_timestamp(config.timestamp_precision)
    local points = points_tags_tables(config)

    for name, value in pairs(points) do
        value = tostring(value)
        -- Only add metrics that are originally integers or floats, as that is
        -- what Carbon is limited to storing.
        if string.match(value, "^[%d.]+$") then
            table.insert(api_message, name.." "..value.." "..message_timestamp)
        end
    end

    return api_message
end

function influxdb_line_msg(config)
    local api_message = {}
    local message_timestamp = field_util.message_timestamp(config.timestamp_precision)
    local points, tags = points_tags_tables(config)

    -- Build a table of data points that we will eventually convert
    -- to a newline delimited list of InfluxDB write API line protocol
    -- formatted values that are then injected back into the pipeline.
    for name, value in pairs(points) do
        -- Wrap in double quotes and escape embedded double quotes
        -- as defined by the protocol.
        if type(value) == "string" then
            value = '"'..value:gsub('"', '\\"')..'"'
        end

        -- Always send numbers as formatted floats, so InfluxDB will accept
        -- them if they happen to change from ints to floats between
        -- points in time.  Forcing them to always be floats avoids this.
        -- Use the decimal_precision config option to control the
        -- numbers after the decimal that are printed.
        if type(value) == "number" or string.match(value, "^[%d.]+$") then
            value = string.format("%."..config.decimal_precision.."f", value)
        end

        -- Format the line differently based on the presence of tags
        -- i.e. length of the tags table is > 0
        if tags and #tags > 0 then
            insert_value = string.format("%s,%s %s=%s %d", influxdb_kv_fmt(name), table.concat(tags, ","),
                                         config.value_field_key, value, message_timestamp)
            table.insert(api_message, insert_value)
        else
            insert_value = string.format("%s %s=%s %d", influxdb_kv_fmt(name), config.value_field_key, value,
                                         message_timestamp)
            table.insert(api_message, insert_value)
        end
    end

    return api_message
end

function set_config(client_config)
    -- Initialize table with default values for ts_line_protocol module
    local module_config = {
        carbon_format = false,
        decimal_precision = "6",
        name_prefix = false,
        name_prefix_delimiter = false,
        skip_fields_str = false,
        source_value_field = false,
        tag_fields_str = "**all_base**",
        timestamp_precision = "ms",
        value_field_key = "value"
    }

    -- Update module_config defaults with those found in client configs
    for option in pairs(module_config) do
        if client_config[option] then
            module_config[option] = client_config[option]
        end
    end

    -- Remove blacklisted fields from the set of base fields that we use, and
    -- create a table of dynamic fields to skip.
    if module_config.skip_fields_str then
        module_config.skip_fields,
        module_config.skip_fields_all_base = field_util.field_map(client_config.skip_fields_str)
        module_config.used_base_fields = field_util.used_base_fields(module_config.skip_fields)
    end

    -- Create and populate a table of fields to be used as tags
    if module_config.tag_fields_str then
        module_config.used_tag_fields,
        module_config.tag_fields_all_base,
        module_config.tag_fields_all = field_util.field_map(client_config.tag_fields_str)
    end

    -- Cache whether or not name_prefix needs interpolation
    module_config.interp_name_prefix = false
    if module_config.name_prefix and string.find(module_config.name_prefix, "%%{[%w%p]-}") then
        module_config.interp_name_prefix = true
    end

    return module_config
end

return M


-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to JSON for InfluxDB HTTP API. Includes
all standard message fields and iterates through all of the dynamically
specified fields, skipping any bytes fields or any fields explicitly omitted
using the `skip_fields` config option.

.. note::
    This encoder is intended for use with InfluxDB versions *prior* to 0.9. If
    you're working with InfluxDB v0.9 or greater, you'll want to use the
    :ref:`config_schema_influx_write_encoder` instead.

Config:

- series (string, optional, default "series")
    String to use as the `series` key's value in the generated JSON. Supports
    interpolation of field values from the processed message, using
    `%{fieldname}`. Any `fieldname` values of "Type", "Payload", "Hostname",
    "Pid", "Logger", "Severity", or "EnvVersion" will be extracted from the
    the base message schema, any other values will be assumed to refer to a
    dynamic message field. Only the first value of the first instance of a
    dynamic message field can be used for series name interpolation. If the
    dynamic field doesn't exist, the uninterpolated value will be left in the
    series name. Note that it is not possible to interpolate either the
    "Timestamp" or the "Uuid" message fields into the series name, those
    values will be interpreted as referring to dynamic message fields.

- skip_fields (string, optional, default "")
    Space delimited set of fields that should *not* be included in the
    InfluxDB records being generated. Any fieldname values of "Type",
    "Payload", "Hostname", "Pid", "Logger", "Severity", or "EnvVersion" will
    be assumed to refer to the corresponding field from the base message
    schema. Any other values will be assumed to refer to a dynamic message
    field.

- multi_series (boolean, optional, default false)
    Instead of submitting all fields to InfluxDB as attributes of a single
    series, submit a series for each field that sets a "value" attribute to
    the value of the field.  This also sets the name attribute to the series
    value with the field name appended to it by a ".". This is the recommended
    by InfluxDB for v0.9 onwards as it is found to provide better performance
    when querying and aggregating across multiple series.

- exclude_base_fields (boolean, optional, default false)
    Don't send the base fields to InfluxDB.  This saves storage space by not
    including base fields that are mostly redundant and unused data. If 
    skip_fields includes base fields, this overrides it and will only be
    relevant for skipping dynamic fields.

*Example Heka Configuration*

.. code-block:: ini

    [influxdb]
    type = "SandboxEncoder"
    filename = "lua_encoders/schema_influx.lua"
        [influxdb.config]
        series = "heka.%{Logger}"
        skip_fields = "Pid EnvVersion"

    [InfluxOutput]
    message_matcher = "Type == 'influxdb'"
    encoder = "influxdb"
    type = "HttpOutput"
    address = "http://influxdbserver.example.com:8086/db/databasename/series"
    username = "influx_username"
    password = "influx_password"

*Example Output*

.. code-block:: json

    [{"points":[[1.409378221e+21,"log","test","systemName","TcpInput",5,1,"test"]],"name":"heka.MyLogger","columns":["Time","Type","Payload","Hostname","Logger","Severity","syslogfacility","programname"]}]

--]=]

require "cjson"
require "string"

local series_orig  = read_config("series") or "series"
local series = series_orig
local use_subs
if string.find(series, "%%{[%w%p]-}") then
    use_subs = true
end

-- Default this option to false
local multi_series = read_config("multi_series") or false

-- Default this option to false
local exclude_base_fields = read_config("exclude_base_fields") or false

local base_fields_map = {
    Type = true,
    Payload = true,
    Hostname = true,
    Pid = true,
    Logger = true,
    Severity = true,
    EnvVersion = true
}

local base_fields_list = {
    "Type",
    "Payload",
    "Hostname",
    "Pid",
    "Logger",
    "Severity",
    "EnvVersion"
}

-- Used for interpolating message fields into series name.
local function sub_func(key)
    if base_fields_map[key] then
        return read_message(key)
    else
        local val = read_message("Fields["..key.."]")
        if val then
            return val
        end
        return "%{"..key.."}"
    end
end

-- Remove blacklisted fields from the set of base fields that we use, and
-- create a table of dynamic fields to skip.
local used_base_fields = {}
local skip_fields_str = read_config("skip_fields")
local skip_fields = {}
if skip_fields_str then
    for field in string.gmatch(skip_fields_str, "[%S]+") do
        skip_fields[field] = true
    end
    for _, base_field in ipairs(base_fields_list) do
        if not skip_fields[base_field] then
            used_base_fields[#used_base_fields+1] = base_field
        else
            skip_fields[base_field] = nil
        end
    end
else
    used_base_fields = base_fields_list
end

local function get_array_value(field, field_idx, count)
    local value = {}
    for i = 1,count do
        value[i] = read_message("Fields["..field.."]",field_idx,i-1)
    end
    return value
end

local function process_single_series(columns, values, place)
    local seen = {}
    local seen_count
    while true do
        local typ, name, value, representation, count = read_next_field()
        if not typ then break end

        if name ~= "Timestamp" and typ ~= 1 then -- exclude bytes
            if not skip_fields_str or not skip_fields[name] then
                seen_count = seen[name]
                if not seen_count then
                    columns[place] = name
                    seen[name] = 1
                    seen_count = 1
                else
                    seen_count = seen_count + 1
                    seen[name] = seen_count
                    columns[place] = name..tostring(seen_count)
                end
                if count == 1 then
                    values[place] = value
                else
                    values[place] = get_array_value(name, seen_count-1, count)
                end
                place = place + 1
            end
        end
    end

    if use_subs then
        series = string.gsub(series_orig, "%%{([%w%p]-)}", sub_func)
    end

    local output = {
        {
            name = series,
            columns = columns,
            points =  {values}
        }
    }
    return output
end

local function process_multi_series(columns, values, place)
    local output_index = 1

    if use_subs then
        series = string.gsub(series_orig, "%%{([%w%p]-)}", sub_func)
    else
        series = series_orig
    end

    local output = {}
    while true do
        -- Iterate through Fields array in the message
        local typ, name, value, representation, count = read_next_field()
        -- Exit the perpetual loop when the iteration is complete
        if not typ then break end

        -- Only process fields that are not requested to be skipped
        if not skip_fields_str or not skip_fields[name] then
            local field_name = ""
            local field_columns = {}
            local field_points = {}

            -- Instantiate a table in the output table for this iteration
            output[output_index] = {}

            -- Set the name attribute of this table by concatenating series
            -- with the name of this particular field
            field_name = series.."."..name

            -- Create new tables built from local columns, values in this table
            for k,v in pairs(columns) do field_columns[k] = v end
            for k,v in pairs(values) do field_points[k] = v end

            -- Merge added values to each table for the field in this iteration
            field_columns[place] = "value"
            field_points[place] = value
            -- Structure the table to match the expected Influxdb structure
            output[output_index] = {
                name = field_name,
                columns = field_columns,
                points = {field_points}
            }
            -- Increment the index to the table so the next iteration will
            -- append another entry to the array. This allows us to send a
            -- bunch of metrics to InfluxDB with a single HTTP request
            output_index = output_index + 1
        end
    end

    return output
end

function process_message()
    local columns = {}
    local values = {}

    columns[1] = "time" -- InfluxDB's default
    values[1] = read_message("Timestamp") / 1e6

    local place = 2
    if not exclude_base_fields then
        for _, field in ipairs(used_base_fields) do
            columns[place] = field
            values[place] = read_message(field)
            place = place + 1
        end
    end

    local output
    if multi_series then
        output = process_multi_series(columns, values, place)
    else
        output = process_single_series(columns, values, place)
    end

    inject_payload("json", "influx_message", cjson.encode(output))
    return 0
end

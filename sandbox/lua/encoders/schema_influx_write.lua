-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to JSON for InfluxDB HTTP write API 
(changed in InfluxDB v0.9.0).  Includes all standard message fields and
iterates through all of the dynamically specified fields, skipping any
bytes fields or any fields explicitly omitted using the `skip_fields`
config option.  It can also map Heka message fields as tags in the request
sent to the InfluxDB API.

Config:

- name_prefix (string, optional, default "name")
    String to use as the `name` key's prefix value in the generated JSON.
    Supports interpolation of field values from the processed message, using
    `%{fieldname}`. Any `fieldname` values of "Type", "Payload", "Hostname",
    "Pid", "Logger", "Severity", or "EnvVersion" will be extracted from the
    the base message schema, any other values will be assumed to refer to a
    dynamic message field. Only the first value of the first instance of a
    dynamic message field can be used for name name interpolation. If the
    dynamic field doesn't exist, the uninterpolated value will be left in the
    name. Note that it is not possible to interpolate either the
    "Timestamp" or the "Uuid" message fields into the name, those
    values will be interpreted as referring to dynamic message fields.

- database (string, required, default "mydb")
    The InfluxDB database to store the metrics in.  Only relevant when
    post_v09_api is true.

- retention_policy (string, optional, default "")
    The InfluxDB retentionPoilcy value for the database in which the
    metrics are being sent to.  Only relevant when post_v09_api is true.

- skip_fields (string, optional, default "")
    Space delimited set of fields that should *not* be included in the
    InfluxDB records being generated. Any fieldname values of "Type",
    "Payload", "Hostname", "Pid", "Logger", "Severity", or "EnvVersion" will
    be assumed to refer to the corresponding field from the base message
    schema. Any other values will be assumed to refer to a dynamic message
    field. The magic value "**all_base**" can be used to exclude base fields
    from being mapped to the event altogether (useful if you don't want to 
    use tags and embed them in the name_prefix instead).

- tag_fields (string, optional, default "**all_base**")
    Take fields defined and adds them as tags of the point(s) sent to
    InfluxDB for the message.  The magic values "**all**" and "**all_base**"
    are used to map all fields (including base) to tags and only base fields
    to tags, respectively.  If those magic values aren't used, then only those
    fields defined will map to tags of the points sent to InfluxDB.

*Example Heka Configuration*

.. code-block:: ini

    [influxdb]
    type = "SandboxEncoder"
    filename = "lua_encoders/schema_influx_write.lua"
        [influxdb.config]
        name_prefix = "heka.%{Logger}"
        database = "mydb"
        retention_policy = "mypolicy"
        skip_fields = "Pid EnvVersion"
        tag_fields = "Hostname Type"

    [InfluxOutput]
    message_matcher = "Type == 'influxdb'"
    encoder = "influxdb"
    type = "HttpOutput"
    address = "http://influxdbserver.example.com:8086/write"
    username = "influx_username"
    password = "influx_password"

-- TODO --
*Example Output*

.. code-block:: json

    [{"points":[[1.409378221e+21,"log","test","systemName","TcpInput",5,1,"test"]],"name":"heka.MyLogger","columns":["Time","Type","Payload","Hostname","Logger","Severity","syslogfacility","programname"]}]

--]=]
-- END TODO --

require "cjson"
require "string"

local name_prefix_orig  = read_config("name_prefix") or "name"
local name_prefix = name_prefix_orig
local use_subs
if string.find(name_prefix, "%%{[%w%p]-}") then
    use_subs = true
end

-- Default this option to "mydb"
local influxdb_db = read_config("database") or "mydb"

-- Default this option to ""
local retention_policy = read_config("retention_policy") or ""

-- Default this option to ""
local tag_fields = read_config("tag_fields") or ""

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

local base_fields_tag_list = {
    "Type",
    "Hostname",
    "Severity",
    "Logger"
}

-- Used for interpolating message fields into name_prefix.
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

function process_message()
    local columns = {}
    local values = {}
    local message_timestamp = read_message("Timestamp") / 1e6

    columns[1] = "time" -- InfluxDB's default
    values[1] = message_timestamp

    local place = 2
    if not exclude_base_fields then
        for _, field in ipairs(used_base_fields) do
            columns[place] = field
            values[place] = read_message(field)
            place = place + 1
        end
    end

    local output_index = 1

    if use_subs then
        name_prefix = string.gsub(name_prefix_orig, "%%{([%w%p]-)}", sub_func)
    else
        name_prefix = name_prefix_orig
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

            -- Set the name attribute of this table by concatenating name_prefix
            -- with the name of this particular field
            field_name = name_prefix.."."..name

            -- Create new tables built from local columns, values in this table
            for k,v in pairs(columns) do field_columns[k] = v end
            for k,v in pairs(values) do field_points[k] = v end

            -- Merge added values to each table for the field in this iteration
            field_columns[place] = "value"
            field_points[place] = value

            -- Structure the table to match the expected Influxdb structure
            output[output_index] = {
                name = field_name,
                points = {field_columns[place] = field_points[place]}
            }

            -- Increment the index to the table so the next iteration will
            -- append another entry to the array. This allows us to send a
            -- bunch of metrics to InfluxDB with a single HTTP request
            output_index = output_index + 1
        end
    end

    api_message = {
        database = influxdb_db,
        retentionPolicy = retention_policy,
        tags = message_tags,
        timestamp = message_timestamp,
        points = output
    }

    inject_payload("json", "influx_message", cjson.encode(api_message))

    return 0
end

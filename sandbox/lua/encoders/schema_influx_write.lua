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

- skip_fields (string, optional, default nil)
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

- timestamp_precision (string, optional, default "n")
    Specify the timestamp precision that you want the event sent with.  The
    default is to use nanoseconds by dividing the Heka message timestamp
    by 1000000, but this math can be altered by specifying one of the precision
    values supported by the InfluxDB write API (n, u, ms, s, m, h).  The default
    of nanoseconds is used to match the precision of Timestamp in Heka messages.

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
require "math"
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

-- Default this option to "ms"
local timestamp_precision = read_config("timestamp_precision") or "ms"

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

local base_fields_tag_map = {
    Type = true,
    Hostname = true,
    Severity = true,
    Logger = true
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
        if skip_fields["**all_base**"] then
            skip_fields[base_field] = true
        elseif not skip_fields[base_field] then
            used_base_fields[#used_base_fields+1] = base_field
        else
            skip_fields[base_field] = nil
        end
    end
else
    used_base_fields = base_fields_list
end

-- Create and populate a table of fields to be used as tags
local tag_fields_str = read_config("tag_fields") or "**all_base**"
local used_tag_fields = {}
if tag_fields_str then
    for field in string.gmatch(tag_fields_str, "[%S]+") do
        used_tag_fields[field] = true
    end
end

function process_message()
    local columns = {}
    local values = {}

    -- Determine the precision of the timestamp and do the math
    local timestamp_divisor = 1
    if timestamp_precision == "u" then
        timestamp_divisor = 1e3
    elseif timestamp_precision == "ms" then
        timestamp_divisor = 1e6
    elseif timestamp_precision == "s" then
        timestamp_divisor = 1e9
    elseif timestamp_precision == "m" then
        timestamp_divisor = 1e9 * 60
    elseif timestamp_precision == "h" then
        timestamp_divisor = 1e9 * 60 * 60
    end
    local message_timestamp = math.floor(read_message("Timestamp") / timestamp_divisor)

    -- Create a counter for the table indexes for columns and values
    -- then populate those tables with the used base fields previously
    -- determined
    local place = 1
    for _, field in ipairs(used_base_fields) do
        columns[place] = field
        values[place] = read_message(field)
        place = place + 1
    end

    -- If the %{field} substitutions are defined in the name_prefix,
    -- replace them with the actual values from the message here
    if use_subs then
        name_prefix = string.gsub(name_prefix_orig, "%%{([%w%p]-)}", sub_func)
    else
        name_prefix = name_prefix_orig
    end

    -- Initialize and populate the table of tags to include in the 
    -- InfluxDB write API message
    local message_tags = {}
    for _, field in ipairs(base_fields_list) do
        if (used_tag_fields["**all_base**"] or used_tag_fields["**all_base**"])
            and base_fields_tag_map[field] then
            message_tags[field] = read_message(field)
        elseif used_tag_fields[field] and base_fields_tag_map[field] then
            message_tags[field] = read_message(field)
        end
    end

    -- Initialize a counter for the InfluxDB write API message data points
    local points_index = 1

    -- Initialize the table of data points and populate it with data
    -- from the Heka message.  When skip_fields includes "**all_base**",
    -- Only dynamic fields are included as InfluxDB data points, while
    -- the base fields serve as tags for them. If skip_fields does not
    -- define any base fields, they are added to the fields of each data
    -- point and each dynamic field value is set as the "value" field.
    -- Adding "**all_base**" is recommended to avoid redundant data being
    -- stored in each data point (the base fields as fields and as tags).
    local points = {}
    while true do
        -- Iterate through Fields array in the message
        local typ, name, value, representation, count = read_next_field()
        -- Exit the perpetual loop when the iteration is complete
        if not typ then break end

        -- Only process fields that are not requested to be skipped
        if not skip_fields_str or not skip_fields[name] then
            -- Set the name attribute of this table by concatenating name_prefix
            -- with the name of this particular field
            local field_name = name_prefix.."."..name

            -- Merge existing base fields with field in this iteration
            local fields = {}
            for k,v in pairs(columns) do fields[v] = values[k] end

            -- Structure the table to match the expected Influxdb structure
            fields["value"] = value
            points[points_index] = {
                name = field_name,
                fields = fields
            }

            -- Increment the index to the table so the next iteration will
            -- append another entry to the array. This allows us to send a
            -- bunch of metrics to InfluxDB with a single HTTP request
            points_index = points_index + 1
        end

        -- Include the dynamic fields as tags if they are defined in
        -- configuration or the magic value "**all**" is defined.
        if used_tag_fields[name] or used_tag_fields["**all**"] then
            message_tags[name] = value
        end
    end

    api_message = {
        database = influxdb_db,
        points = points,
        precision = timestamp_precision,
        retentionPolicy = retention_policy,
        timestamp = message_timestamp,
        tags = message_tags
    }

    inject_payload("json", "influx_message", cjson.encode(api_message))

    return 0
end


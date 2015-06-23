-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to JSON for InfluxDB HTTP write API (new
in InfluxDB v0.9.0). Optionally includes all standard message fields as tags
or fields and iterates through all of the dynamic fields to add as points
(series), skipping any fields explicitly omitted using the `skip_fields`
config option.  It can also map any Heka message fields as tags in the request
sent to the InfluxDB API, using the `tag_fields` config option. Tags,
timestamp/precision, database, and retention policy are defined once for all
points submitted in the API call.  All dynamic fields in the Heka message are
converted to separate series in the `points` array that is submitted to
InfluxDB.

.. note::
    This encoder is intended for use with InfluxDB versions 0.9 or greater. If
    you're working with InfluxDB versions prior to 0.9, you'll want to use the
    :ref:`config_schema_influx_encoder` instead.

Config:

- name_prefix (string, optional, default "name")
    String to use as the `name` key's prefix value in the generated JSON.
    Supports :ref:`message field interpolation<sandbox_msg_interpolate_module>`.
    `%{fieldname}`. Any `fieldname` values of "Type", "Payload", "Hostname",
    "Pid", "Logger", "Severity", or "EnvVersion" will be extracted from the
    the base message schema, any other values will be assumed to refer to a
    dynamic message field. Only the first value of the first instance of a
    dynamic message field can be used for name name interpolation. If the
    dynamic field doesn't exist, the uninterpolated value will be left in the
    name. Note that it is not possible to interpolate either the
    "Timestamp" or the "Uuid" message fields into the name, those
    values will be interpreted as referring to dynamic message fields.

- database (string, required)
    The InfluxDB database to store the metrics in.

- retention_policy (string, optional, default "")
    The InfluxDB retentionPolicy value for the database in which the
    metrics are being sent to. When an empty value for retentionPolicy is
    sent to InfluxDB, the default policy is used.  The default policy uses
    infinite retention.  See InfluxDB 0.9.0 docs for more information.

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
    are used to map all non-numeric fields (including base) to tags and only
    base fields to tags, respectively.  If those magic values aren't used,
    then only those fields defined will map to tags of the points sent to InfluxDB.
    The tag_fields values are independent of the skip_fields values and have
    no affect on each other.  You can skip fields from being sent to InfluxDB
    as points (metrics), but still include them as tags, as long as they are not
    numbers (those are filtered out automatically).

- timestamp_precision (string, optional, default "ms")
    Specify the timestamp precision that you want the event sent with.  The
    default is to use milliseconds by dividing the Heka message timestamp
    by 1e6, but this math can be altered by specifying one of the precision
    values supported by the InfluxDB write API (ms, s, m, h). Other precisions
    supported by InfluxDB of n and u are not yet supported.

*Example Heka Configuration*

.. code-block:: ini

    [LoadAvgPoller]
    type = "FilePollingInput"
    ticker_interval = 5
    file_path = "/proc/loadavg"
    decoder = "LinuxStatsDecoder"

    [LoadAvgDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_loadavg.lua"

    [LinuxStatsDecoder]
    type = "MultiDecoder"
    subs = ["LoadAvgDecoder", "AddStaticFields"]
    cascade_strategy = "all"
    log_sub_errors = false

    [AddStaticFields]
    type = "ScribbleDecoder"

        [AddStaticFields.message_fields]
        Environment = "DEV"

    [InfluxdbOutput]
    type = "HttpOutput"
    message_matcher = "Type =~ /stats.*/"
    encoder = "InfluxdbEncoder"
    address = "http://influxdbserver.example.com:8086/write"
    username = "influx_username"
    password = "influx_password"

    [InfluxdbEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/schema_influx_write.lua"

        [InfluxdbEncoder.config]
        name_prefix = "%{Hostname}.%{Type}"
        skip_fields = "**all_base** FilePath NumProcesses Environment"
        tag_fields = "Hostname Environment"
        timestamp_precision= "s"

*Example Output*

.. code-block:: json

    {"database":"mydb","retentionPolicy":"","tags":{"Environment":"DEV","Hostname":"my_hostname"}, "points":[{"measurement":"my_hostname.stats.loadavg.5MinAvg","fields":{"value":0.03}},{"measurement":"my_hostname.stats.loadavg.15MinAvg","fields":{"value":0.05}},{"measurement":"my_hostname.stats.loadavg.1MinAvg","fields":{"value":0.01}}],"timestamp":1426439735,"precision":"s"}

--]=]

require "cjson"
require "math"
require "string"
local interp = require "msg_interpolate"

local name_prefix_orig  = read_config("name_prefix") or "name"
local name_prefix = name_prefix_orig
local use_subs
if string.find(name_prefix, "%%{[%w%p]-}") then
    use_subs = true
end

-- Required field that errors when not defined
local influxdb_db = read_config("database") or error("`database` setting required")

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

-- Remove blacklisted fields from the set of base fields that we use, and
-- create a table of dynamic fields to skip.
local used_base_fields = {}
local skip_fields_str = read_config("skip_fields")
local skip_fields = {}
local skip_fields_all_base = false
if skip_fields_str then
    for field in string.gmatch(skip_fields_str, "[%S]+") do
        skip_fields[field] = true
        if field == "**all_base**" then
            skip_fields_all_base = true
        end
    end

    for _, base_field in ipairs(base_fields_list) do
        if skip_fields_all_base then
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
local tag_fields_all_base = false
local tag_fields_all = false
if tag_fields_str then
    for field in string.gmatch(tag_fields_str, "[%S]+") do
        used_tag_fields[field] = true
        if field == "**all_base**" then
            tag_fields_all_base = true
        end
        if field == "**all**" then
            tag_fields_all = true
        end
    end
end

-- Determine the precision of the timestamp and do the math
local timestamp_divisor = 1e6
if timestamp_precision == "s" then
    timestamp_divisor = 1e9
elseif timestamp_precision == "m" then
    timestamp_divisor = 1e9 * 60
elseif timestamp_precision == "h" then
    timestamp_divisor = 1e9 * 60 * 60
end

function process_message()
    local columns = {}
    local values = {}

    local message_timestamp = read_message("Timestamp")
    message_timestamp = math.floor(message_timestamp / timestamp_divisor)

    -- Create a counter for the table indexes for columns and values
    -- then populate those tables with the used base fields previously
    -- determined
    for place, field in ipairs(used_base_fields) do
        columns[place] = field
        values[place] = read_message(field)
    end

    -- If the %{field} substitutions are defined in the name_prefix,
    -- replace them with the actual values from the message here
    if use_subs then
        name_prefix = interp.interpolate_from_msg(name_prefix_orig, nil)
    else
        name_prefix = name_prefix_orig
    end

    -- Initialize and populate the table of tags to include in the
    -- InfluxDB write API message
    -- Convert value to a string as this is required by the API
    local message_tags = {}
    for _, field in ipairs(base_fields_list) do
        local base_field_value = read_message(field)
        if (tag_fields_all or tag_fields_all_base)
            and base_fields_tag_map[field] and type(base_field_value) ~= "number" then
            message_tags[field] = tostring(base_field_value)
        elseif used_tag_fields[field] and base_fields_tag_map[field]
            and type(base_field_value) ~= "number" then
            message_tags[field] = tostring(base_field_value)
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
                measurement = field_name,
                fields = fields
            }

            -- Increment the index to the table so the next iteration will
            -- append another entry to the array. This allows us to send a
            -- bunch of metrics to InfluxDB with a single HTTP request
            points_index = points_index + 1
        end

        -- Include the dynamic fields as tags if they are defined in
        -- configuration or the magic value "**all**" is defined.
        -- Convert value to a string as this is required by the API
        if (tag_fields_all or used_tag_fields[name])
            and type(value) ~= "number" then
            message_tags[name] = tostring(value)
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


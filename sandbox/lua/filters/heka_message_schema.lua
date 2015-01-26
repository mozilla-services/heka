-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Generates documentation for each unique message in a data stream.  The output is
a hierarchy of Logger, Type, EnvVersion, and a list of associated message field
attributes including their counts (number in the brackets). This plugin is meant
for data discovery/exploration and should not be left running on a production
system.

Config:

<none>

*Example Heka Configuration*

.. code-block:: ini

    [SyncMessageSchema]
    type = "SandboxFilter"
    filename = "lua_filters/heka_message_schema.lua"
    ticker_interval = 60
    preserve_data = false
    message_matcher = "Logger != 'SyncMessageSchema' && Logger =~ /^Sync/"

*Example Output*

| Sync-1_5-Webserver [54600]
|     slf [54600]
|          -no version- [54600]
|             upstream_response_time (mismatch)
|             http_user_agent (string)
|             body_bytes_sent (number)
|             remote_addr (string)
|             request (string)
|             upstream_status (mismatch)
|             status (number)
|             request_time (number)
|             request_length (number)
| Sync-1_5-SlowQuery [37]
|     mysql.slow-query [37]
|          -no version- [37]
|             Query_time (number)
|             Rows_examined (number)
|             Rows_sent (number)
|             Lock_time (number)
--]]

schema  = {}

local cnt_key = "_cnt_"

local function get_type(t)
    if t == -1 then
        return "mismatch"
    elseif t == 0  or t == 1 then
        return "string"
    elseif t == 2  or t == 3 then
        return "number"
    elseif t == 4 then
       return "bool"
    end
    return "unknown"
end

local function get_table(t, key)
    local v = t[key]
    if not v then
        v = {}
        v[cnt_key] = 0
        t[key] = v
    end
    v[cnt_key] = v[cnt_key] + 1

    return v
end

function process_message ()
    local l = get_table(schema, read_message("Logger"))
    local t = get_table(l, read_message("Type"))
    local v = get_table(t, read_message("EnvVersion"))

    while true do
        local typ, name, value, representation, count = read_next_field()
        if not typ then break end

        if v[name] then
            v[name][cnt_key] = v[name][cnt_key] + 1
            if typ ~= v[name].type then
                v[name].type = -1 -- mis-matched types
            end
        else
            v[name] = {[cnt_key] = 1, type = typ}
        end
    end
    return 0
end

local function output_fields(t, cnt)
    for k, v in pairs(t) do
        if k ~= cnt_key then
            add_to_payload("            ", k, " (", get_type(v.type))
            if cnt ~= v[cnt_key] then
                add_to_payload(" - optional [", v[cnt_key], "]")
            end
            add_to_payload(")\n")
        end
    end
end

local function output_versions(t)
    for k, v in pairs(t) do
        if type(v) == "table" then
            local cnt = v[cnt_key]
            if k == "" then
                k = "-no version-"
            end
            add_to_payload("         ", k, " [", cnt, "]\n")
            output_fields(v, cnt)
        end
    end
end

local function output_types(t)
    for k, v in pairs(t) do
        if type(v) == "table" then
            if k == "" then
                k = "-no type-"
            end
            add_to_payload("    ", k, " [", v[cnt_key], "]\n")
            output_versions(v)
        end
    end
end

local function output_loggers(schema)
    for k, v in pairs(schema) do
        if type(v) == "table" then
            if k == "" then
                k = "-no logger-"
            end
            add_to_payload(k, " [", v[cnt_key], "]\n")
            output_types(v)
        end
    end
end

function timer_event(ns)
    add_to_payload("Logger -> Type -> EnvVersion -> Field Attributes.\n",
           "The number in brackets is the number of occurrences of each logger/type/attribute.\n\n")
    output_loggers(schema)
    inject_payload("txt", "Message Schema")
end

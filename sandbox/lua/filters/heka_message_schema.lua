-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Generates documentation for each message type in a data stream.  The output
includes each message Type, its associated field attributes, and their
counts (number in the brackets). This plugin is meant for data
discovery/exploration and should not be left running on a production system.

Config:

<none>

*Example Heka Configuration*

.. code-block:: ini

    [FxaAuthServerMessageSchema]
    type = "SandboxFilter"
    script_type = "lua"
    filename = "lua_filters/heka_message_schema.lua"
    ticker_interval = 60
    preserve_data = false
    message_matcher = "Logger == 'fxa-auth-server'"

*Example Output*

|  DB.getToken [11598]
|      id (string)
|      rid (string - optional [2])
|      msg (string)
--]]

messages = {}

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

function process_message ()
    local t = read_message("Type")
    local m = messages[t]
    if not m then
        m = {}
        m[cnt_key] = 0
        messages[t] = m
    end
    m[cnt_key] = m[cnt_key] + 1

    while true do
        local h = {type = 0, name = "", value = 0, representation = "", count = 0, key = ""}
        h.type, h.name, h.value, h.representation, h.count = read_next_field()
        if not h.type then break end
        if m[h.name] then
            m[h.name][cnt_key] = m[h.name][cnt_key] + 1
            if h.type ~= m[h.name].type then
                m[h.name].type = -1 -- mis-matched types
            end
        else
            m[h.name] = {[cnt_key] = 1, type = h.type}
        end
    end
    return 0
end

function timer_event(ns)
    output("Message Types and their associated field attributes. The number in brackets is the number of occurrences of each log type/attribute.\n\n")
    for k,v in pairs(messages) do
        local cnt = v[cnt_key]
         output(k, " [", cnt, "]\n")
         for m,n in pairs(v) do
             if m ~= cnt_key then
                 output("    ", m, " (", get_type(n.type))
                 if cnt ~= n[cnt_key] then
                     output(" - optional [", n[cnt_key], "]")
                 end
                 output(")\n")
             end
        end
    end
    inject_message("txt", "Message Schema")
end

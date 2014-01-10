-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local cbufd = require "cbufd"
require "circular_buffer"
require "cjson"

cbufs = {}

function init_cbuf(payload_name, data)
    local h = cjson.decode(data.header)
    if not h then
        return nil
    end

    local cb = circular_buffer.new(h.rows, h.columns, h.seconds_per_row)
    for i,v in ipairs(h.column_info) do
        cb:set_header(i, v.name, v.unit, v.aggregation)
    end

    cbufs[payload_name] = {header = h, cbuf = cb}
    return cbufs[payload_name]
end

function process_message ()
    local payload = read_message("Payload")
    local payload_name = read_message("Fields[payload_name]") or ""
    local data = cbufd.grammar:match(payload)
    if not data then
        return -1
    end

    local cb = cbufs[payload_name]
    if not cb then
        cb = init_cbuf(payload_name, data)
        if not cb then
            return -1
        end
    end

    for i,v in ipairs(data) do
        for col, value in ipairs(v) do
            if value == value then -- only aggregrate numbers
                local agg = cb.header.column_info[col].aggregation
                if  agg == "sum" then
                    cb.cbuf:add(v.time, col, value)
                elseif agg == "min" or agg == "max" then
                    cb.cbuf:set(v.time, col, value)
                end
            end
        end
    end
    return 0
end

function timer_event(ns)
    for k,v in pairs(cbufs) do
        inject_message(v.cbuf, k)
    end
end

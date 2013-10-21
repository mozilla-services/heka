-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require("cjson")
local l = require("lpeg")
l.locale(l)

-- sample cbufd
-- {"time":1379574900,"rows":1440,"columns":2,"seconds_per_row":60,"column_info":[{"name":"Requests","unit":"count","aggregation":"sum"},{"name":"Total_Size","unit":"KiB","aggregation":"sum"}]}
-- 1379660520	12075	159901
-- 1379660280	11837	154880

-- cbufd grammar
local eol = l.P"\n"
local header = l.Cg((1 - eol)^1, "header") * eol
local timestamp = l.digit^1 / "%0000000000" / tonumber
local sign = l.P"-"
local float = l.digit^1 * "." * l.digit^1
local number = l.C(sign^-1 * (float + l.digit^1)) / tonumber
local row = l.Ct(l.Cg(timestamp, "time") * ("\t" * number)^1 * eol)
local cbufd = l.Ct(header * row^1) * -1

-- table produced by the grammar
--1
--    1=12075 (number)
--    2=159901 (number)
--    time=1379660520000000000 (number)
--
--2
--    1=11837 (number)
--    2=154880 (number)
--    time=1379660280000000000 (number)
--
--header={"time":1379574900,"rows":1440,"columns":2,"seconds_per_row":60,"column_info":[{"name":"Requests","unit":"count","aggregation":"sum"},{"name":"Total_Size","unit":"KiB","aggregation":"sum"}]}

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
    local data = cbufd:match(payload)
    if not data then
        return 0
    end

    local cb = cbufs[payload_name]
    if not cb then
        cb = init_cbuf(payload_name, data)
        if not cb then
            return 0
        end
    end

    for i,v in ipairs(data) do
        for col, value in ipairs(v) do
            local agg = cb.header.column_info[col].aggregation
            if  agg == "sum" then
                cb.cbuf:add(v.time, col, value)
            elseif agg == "min" then
                local val = cb.cbuf:get(v.time, col)
                if val == nil or value < val then
                    cb.cbuf:set(v.time, col, value)
                end
            elseif agg == "max" then
                local val = cb.cbuf:get(v.time, col)
                if val == nil or value > val then
                    cb.cbuf:set(v.time, col, value)
                end
            end
            -- cannot aggregate avg or none
        end
    end
    return 0
end

function timer_event(ns)
    for k,v in pairs(cbufs) do
        inject_message(v.cbuf, k)
    end
end

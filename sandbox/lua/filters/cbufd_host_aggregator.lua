-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local cbufd = require "cbufd"
require "circular_buffer"
require "cjson"
require "string"
require "table"

local host_expiration = (read_config("host_expiration") or 120) * 1e9
local rows = read_config("rows")
local cols = read_config("max_hosts") -- hosts to preallocate space for

hosts = {}
hosts_size = 0
payloads = {}

function init_payload(hostname, payload_name, data)
    local h = cjson.decode(data.header)
    if not h then
        return nil
    end

    local pl = {header = h, cbufs = {}}
    for i,v in ipairs(h.column_info) do
        local cb = circular_buffer.new(rows, cols, h.seconds_per_row)
        for k,h in pairs(hosts) do
            cb:set_header(h.index, k, v.unit, v.aggregation)
        end
        table.insert(pl.cbufs, cb)
    end

    payloads[payload_name] = pl
    return pl
end

function process_message ()
    local ts = read_message("Timestamp")
    local hostname = read_message("Hostname")
    local payload = read_message("Payload")
    local payload_name = read_message("Fields[payload_name]") or ""
    local data = cbufd.grammar:match(payload)
    if not data then
        return -1
    end

    host = hosts[hostname]
    if not host then
        if hosts_size == rows then
            for k,v in pairs(hosts) do
                if ts - v.last_update >= host_expiration then
                    -- leave the cbuf data intact
                    -- assume this instance is just a replacement
                    -- for the instance that timed out
                    v.last_update = ts
                    hosts[hostname] = v
                    host = v
                    hosts[k] = nil
                    break
                end
            end
            if not host then
                error("The number of hosts exceeds the 'max_hosts' configuration")
            end
        else
            hosts_size = hosts_size + 1
            hosts[hostname] = {last_update = ts, index = hosts_size}
            host = hosts[hostname]
        end

        for k,v in pairs(payloads) do
            for i, cb in ipairs(v.cbufs) do
                cb:set_header(host.index, v.header.column_info[i].unit, hostname)
            end
        end
    end

    local pl = payloads[payload_name]
    if not pl then
        pl = init_payload(hostname, payload_name, data)
        if not pl then
            return -1
        end
    end

    if ts > host.last_update then
        host.last_update = ts
    end

    for i,v in ipairs(data) do
        for col, value in ipairs(v) do
            if value == value then -- only aggregrate numbers
                local agg = pl.header.column_info[col].aggregation
                if  agg == "sum" then
                    pl.cbufs[col]:add(v.time, host.index, value)
                elseif agg == "min" or agg == "max" then
                    pl.cbufs[col]:set(v.time, host.index, value)
                end
            end
        end
    end
    return 0
end

function timer_event(ns)
    for k,v in pairs(payloads) do
        for i, cb in ipairs(v.cbufs) do
            inject_message(cb, string.format("%s (%s)", k, v.header.column_info[i].name))
        end
    end
end

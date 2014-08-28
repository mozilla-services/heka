-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Collects the circular buffer delta output from multiple instances of an
upstream sandbox filter (the filters should all be the same version at least
with respect to their cbuf output). Each column from the source circular
buffer will become its own graph. i.e., 'Error Count' will become a graph
with each host being represented in a column.

Config:

- max_hosts (uint)
    Pre-allocates the number of host columns in the graph(s). If the number of
    active hosts exceed this value, the plugin will terminate.

- rows (uint)
    The number of rows to keep from the original circular buffer.  Storing all
    the data from all the hosts is not practical since you will most likely
    run into memory and output size restrictions (adjust the view down as
    necessary).

- host_expiration (uint, optional, default 120 seconds)
    The amount of time a host has to be inactive before it can be replaced by
    a new host.

- preservation_version (uint, optional, default 0)
    If `preserve_data = true` is set in the SandboxFilter configuration, then
    this value should be incremented every time the `max_hosts` or `rows`
    configuration is changed to prevent the plugin from failing to start
    during data restoration.


*Example Heka Configuration*

.. code-block:: ini

    [TelemetryServerMetricsHostAggregator]
    type = "SandboxFilter"
    message_matcher = "Logger == 'TelemetryServerMetrics' && Fields[payload_type] == 'cbufd'"
    ticker_interval = 60
    filename = "lua_filters/cbufd_host_aggregator.lua"
    preserve_data = true

    [TelemetryServerMetricsHostAggregator.config]
    max_hosts = 5
    rows = 60
    host_expiration = 120
    preservation_version = 0
--]]
_PRESERVATION_VERSION = read_config("preservation_version") or 0

local cbufd = require "cbufd"
require "circular_buffer"
require "cjson"
require "string"
require "table"

local host_expiration = (read_config("host_expiration") or 120) * 1e9
local rows = read_config("rows") or error("rows configuration must be specified")
local cols = read_config("max_hosts") or error("max_host configuration must be specified") -- hosts to preallocate space for

hosts = {}
hosts_size = 0
payloads = {}

function init_payload(hostname, payload_name, data)
    local ok, h = pcall(cjson.decode, data.header)
    if not ok then
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

    local host = hosts[hostname]
    if not host then
        if hosts_size == cols then
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
                cb:set_header(host.index, hostname, v.header.column_info[i].unit, v.header.column_info[i].aggregation)
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

local options = '{"options":{"stackedGraph":true,"fillGraph":true}}\n'
function timer_event(ns)
    for k,v in pairs(payloads) do
        for i, cb in ipairs(v.cbufs) do
            inject_payload("cbuf", string.format("%s (%s)", k, v.header.column_info[i].name), options, cb)
        end
    end
end

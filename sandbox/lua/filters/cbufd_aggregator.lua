-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Collects the circular buffer delta output from multiple instances of an upstream
sandbox filter (the filters should all be the same version at least with respect
to their cbuf output). The purpose is to recreate the view at a larger scope in
each level of the aggregation i.e., host view -> datacenter view -> service
level view.

Config:

- enable_delta (bool, optional, default false)
    Specifies whether or not this aggregator should generate cbuf deltas.

- anomaly_config (string, optional)
    A list of anomaly detection specifications.  If not specified no anomaly
    detection/alerting will be performed.

*Example Heka Configuration*

.. code-block:: ini

    [TelemetryServerMetricsAggregator]
    type = "SandboxFilter"
    message_matcher = "Logger == 'TelemetryServerMetrics' && Fields[payload_type] == 'cbufd'"
    ticker_interval = 60
    script_type = "lua"
    filename = "lua_filters/cbufd_aggregator.lua"
    preserve_data = true

    [TelemetryServerMetricsAggregator.config]
    enable_delta = false
    anomaly_config = 'roc("Request Statistics", 1, 15, 0, 1.5, true, false)'
--]]

local alert     = require "alert"
local annotation= require "annotation"
local anomaly   = require "anomaly"
local cbufd     = require "cbufd"
require "circular_buffer"
require "cjson"

local enable_delta = read_config("enable_delta") or false
local anomaly_config = anomaly.parse_config(read_config("anomaly_config"))

cbufs = {}

local function init_cbuf(payload_name, data)
    local h = cjson.decode(data.header)
    if not h then
        return nil
    end

    local cb = circular_buffer.new(h.rows, h.columns, h.seconds_per_row, enable_delta)
    for i,v in ipairs(h.column_info) do
        cb:set_header(i, v.name, v.unit, v.aggregation)
    end
    annotation.set_prune(payload_name, h.rows * h.seconds_per_row * 1e9)

    cbufs[payload_name] = cb
    return cb
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
            if value == value then -- NaN test, only aggregrate numbers
                local n, u, agg = cb:get_header(col)
                if  agg == "sum" then
                    cb:add(v.time, col, value)
                elseif agg == "min" or agg == "max" then
                    cb:set(v.time, col, value)
                end
            end
        end
    end
    return 0
end

function timer_event(ns)
    for k,v in pairs(cbufs) do
        if anomaly_config then
            if not alert.throttled(ns) then
                local msg, annos = anomaly.detect(ns, k, v, anomaly_config)
                if msg then
                    alert.queue_alert(msg)
                    annotation.concat(k, annos)
                end
            end
            output({annotations = annotation.prune(k)}, v)
            inject_message("cbuf", k)
        else
            inject_message(v, k)
        end

        if enable_delta then
            inject_message(v:format("cbufd"), k)
        end
    end
    alert.send_queue(ns)
end

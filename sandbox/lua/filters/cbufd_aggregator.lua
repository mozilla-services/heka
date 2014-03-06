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

- alert_rows (uint, optional, default 15)
    Specifies the size of the rolling average windows to compare. The
    window cannot be more than one third of the entire circular buffer.

- alert_cols (JSON string, optional)
    An array of JSON objects consisting of a 'col' number and a 'deviation'
    alert threshold.  If not specified no anomaly detection/alerting will
    be performed.

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
    alert_rows = 15
    alert_cols =  '[{"col":1, "deviation":2}]'
--]]

local cbufd = require "cbufd"
require "circular_buffer"
require "cjson"
require "math"
require "string"
require "table"

local enable_delta = read_config("enable_delta") or false
local alert_rows = read_config("alert_rows") or 15
local alert_cols = read_config("alert_cols")
if alert_cols then
    alert_cols = cjson.decode(alert_cols)
end

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

    cbufs[payload_name] = {header = h, cbuf = cb, annotations = {}, last_alert = 0}
    return cbufs[payload_name]
end

local alert_message = ""

-- at the moment this is only useful for non-sparse data
local function detect_anomaly(ns, k, v, cols)
    if alert_rows * 3 >= v.header.rows then
        error("alert_rows cannot be more than one third of the circular buffer")
    end
    v.cbuf:add(ns, 1, 0) -- always advance the buffer/graph

    local interval = 1e9 * v.header.seconds_per_row
    local sliding_window = interval * alert_rows
    local previous_window = ns - sliding_window * 2
    local current_window = ns - sliding_window

    for i, c in ipairs(cols) do
        -- Anomaly detection
        -- Compute the average of the last X intervals and the X intervals before
        -- that and compare the difference against the historical standard deviation.
        -- The current interval is not included since it is incomplete and can skew
        -- the stats.
        local historical_sd, hsamples = v.cbuf:compute("sd" , c.col, nil, previous_window - interval)
        if hsamples >= alert_rows then
            local previous_avg, psamples  = v.cbuf:compute("avg", c.col, previous_window, current_window - interval)
            local current_avg, csamples   = v.cbuf:compute("avg", c.col, current_window , ns - interval)

            -- if any sample window doesn't have data an anomaly will not be detected
            -- todo we need to add support for sparse data i.e. failure counts

            -- special case the loss of data anomaly for now
            local loss_of_data = (psamples > 0 and csamples == 0)

            local delta = math.abs(current_avg - previous_avg)
            if delta > historical_sd * c.deviation -- anomaly detected
            and ns - v.last_alert > sliding_window -- hasn't already alerted in the sliding window
            or loss_of_data then
                for m, n in ipairs(v.annotations) do -- clean out old alerts
                    if n.x < (ns - interval * v.header.rows)/1e6 then
                        v.annotations[m] = nil
                    else
                        break
                    end
                end

                v.last_alert = ns - ns % interval
                local msg
                if loss_of_data then
                    msg = string.format("Column %d hasn't received any new data", c.col)
                else
                    msg = string.format("Column %d has fluctuated more than %G standard deviations", c.col, c.deviation)
                end

                table.insert(v.annotations, {x = math.floor(v.last_alert/1e6),
                    col        = c.col,
                    shortText  = "A",
                    text       = msg})

                -- consolidate all alerts into a single message
                alert_message = alert_message .. string.format("%s - %s\n", k, msg)
            end
        end
    end

    output({annotations = v.annotations}, v.cbuf)
    inject_message("cbuf", k)
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
        if alert_cols then
            alert_message = ""
            detect_anomaly(ns, k, v, alert_cols)
        else
            inject_message(v.cbuf, k)
        end
        if enable_delta then
            inject_message(v.cbuf:format("cbufd"), k)
        end
    end

    if alert_message ~= "" then
        output(alert_message)
        inject_message("alert")
    end
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs the disk IO stats on the system running heka.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60 seconds
    per row is a 24 sliding hour window with 1 minute resolution.

*Example Heka Configuration*

.. code-block:: ini

    [DiskStatsFilter]
    type = "SandboxFilter"
    filename = "lua_filters/diskstats.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Type == 'heka.stats.diskstats'"

--]]

require "circular_buffer"
require "string"
local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

local title             = "Disk Stats"
local rows              = read_config("rows") or 1440
local sec_per_row       = read_config("sec_per_row") or 60
local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))
-- annotation.set_prune(title, rows * sec_per_row * 1e9)

local titles = {"Disk Stats", "Time doing IO"}

local count_field_names = {"WritesCompleted", "ReadsCompleted", "SectorsWritten",
    "SectorsRead", "WritesMerged", "ReadsMerged"}

local time_field_names = {"TimeWriting", "TimeReading", "TimeDoingIO", "WeightedTimeDoingIO"}


count_cbuf = circular_buffer.new(rows, #count_field_names, sec_per_row)
time_cbuf = circular_buffer.new(rows, #time_field_names, sec_per_row)
local cbufs = {count_cbuf, time_cbuf}

for i, name in pairs(count_field_names) do
    count_cbuf:set_header(i, name, "Count", "max")
end

for i, name in pairs(time_field_names) do
    time_cbuf:set_header(i, name, "ms", "max")
end

function process_message ()
    local ts = read_message("Timestamp")
    for i, name in pairs(count_field_names) do
        local label = string.format("Fields[%s]", name)
        count_cbuf:set(ts, i, read_message(label))
    end
    for i, name in pairs(time_field_names) do
        local label = string.format("Fields[%s]", name)
        time_cbuf:set(ts, i, read_message(label))
    end
    return 0
end

function timer_event(ns)
    if anomaly_config then
        -- if not alert.throttled(ns) then
        --     local msg, annos = anomaly.detect(ns, title, cbuf, anomaly_config)
        --     if msg then
        --         annotation.concat(title, annos)
        --         alert.send(ns, msg)
        --     end
        -- end
        -- inject_payload("cbuf", title, annotation.prune(title, ns), cbuf)
    else
        for i, cbuf in pairs(cbufs) do
            inject_payload("cbuf", titles[i], cbuf)
        end
    end
end
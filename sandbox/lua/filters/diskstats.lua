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
    preserve_data = true
    message_matcher = "Type == 'heka.stats.diskstats'"

--]]

require "circular_buffer"
require "string"

local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

last_run                = { rows = nil, sec_per_row = nil}
local rows              = read_config("rows") or 1440
local sec_per_row       = nil
local per_unit          = nil

local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))

stats = {
    {
        cbuf = nil,
        fields = {"WritesCompleted", "ReadsCompleted", "SectorsWritten",
                  "SectorsRead", "WritesMerged", "ReadsMerged"},
        msg_fields = {},
        last_value ={},
        title = "Disk Stats",
        unit = "",
        aggregation = "none"
    },
    {
        fields = {"TimeWriting", "TimeReading", "TimeDoingIO", "WeightedTimeDoingIO"},
        msg_fields = {},
        title = "Time doing IO",
        unit = "ms",
        aggregation = "max",
    }
}

for index, cbuf in ipairs(stats) do
    for i, field in ipairs(stats[index].fields) do
        stats[index].msg_fields[i] = string.format("Fields[%s]", field)
    end
end

local function init_cbuf(index)
    local stat = stats[index]
    local cbuf = circular_buffer.new(rows, #stat.fields, sec_per_row)
    for i, label in ipairs(stat.fields) do
        cbuf:set_header(i, label, stat.unit, stat.aggregation)
    end
    annotation.set_prune(stat.title, rows * sec_per_row * 1e9)
    stat.cbuf = cbuf
    -- Set our persisted globals
    last_run.rows = rows
    last_run.sec_per_row = sec_per_row
    return cbuf
end

local function update_sec_per_row()
    sec_per_row = read_message("Fields[TickerInterval]")
    stats[1].unit = "per_" .. sec_per_row .. "_s"
    return sec_per_row
end

-- This tells us if our persisted settings changed between runs.
local function settings_changed()
    update_sec_per_row()
    if rows == last_run.rows and sec_per_row == last_run.sec_per_row then
        return false
    end
    return true
end


function process_message ()
    local ts = read_message("Timestamp")
    if sec_per_row == nil then
        update_sec_per_row()
    end

    local time_stats = stats[2]
    if not time_stats.cbuf then
        init_cbuf(2)
    end

    for i, field in ipairs(time_stats.fields) do
        local val = read_message(time_stats.msg_fields[i])
        if type(val) ~= "number" then return -1 end
        time_stats.cbuf:set(ts, i, val)
    end

    -- Set up our Cbuf if not setup yet
    local index = 1
    local stat = stats[index]
    local cb = stats.cbuf
    if not cb or settings_changed() then
        cb = init_cbuf(index)
    end

    if not cb then return -1 end

    for i, field in ipairs(stat.fields) do
        local val = read_message(stat.msg_fields[i])
        if type(val) ~= "number" then return -1 end
        if stat.last_value[i] ~= nil then
            -- Compute delta
            local delta = val - stat.last_value[i]
            cb:set(ts, i, delta)
        end
        stat.last_value[i] = val
    end

    return 0
end

function timer_event(ns)
    if anomaly_config then
        for i, stat in ipairs(stats) do
            local title = stat.title
            local buf = stat.cbuf
            if not alert.throttled(ns) then
                local msg, annos = anomaly.detect(ns, title, buf, anomaly_config)
                if msg then
                    annotation.concat(buf, annos)
                    alert.send(ns, msg)
                end
            end
            inject_payload("cbuf", title, annotation.prune(title, ns), buf)
        end
    else
        for i, stat in ipairs(stats) do
            inject_payload("cbuf", stat.title, stat.cbuf)
        end
    end
end

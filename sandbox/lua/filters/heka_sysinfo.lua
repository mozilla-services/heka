-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs the Heka system info usage from plugins/sysinfo.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60 seconds
    per row is a 24 sliding hour window with 1 minute resolution.

*Example Heka Configuration*

.. code-block:: ini

    [HekaSysinfo]
    type = "SandboxFilter"
    filename = "lua_filters/heka_sysinfo.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Type == 'heka.sysinfo.sysinfo'"

--]]

local alert      = require "alert"
local annotation = require "annotation"
local anomaly    = require "anomaly"
require "circular_buffer"

local title          = "System Info"
local rows           = read_config("rows") or 1440
local sec_per_row    = read_config("sec_per_row") or 60
local anomaly_config = anomaly.parse_config(read_config("anomaly_config"))
annotation.set_prune(title, rows * sec_per_row * 1e9)

info = circular_buffer.new(rows, 10, sec_per_row)
local LOAD_AVG_1    = info:set_header(1, "LoadAvg1Min", "Avg", "max")
local LOAD_AVG_5    = info:set_header(2, "LoadAvg5Min", "Avg", "max")
local LOAD_AVG_15   = info:set_header(3, "LoadAvg15Min", "Avg", "max")
local TOTAL_RAM     = info:set_header(4, "Totalram", "B", "max")
local FREE_RAM      = info:set_header(5, "Freeram", "B", "max")
local SHARED_RAM    = info:set_header(6, "Sharedram", "count", "max")
local BUFFER_RAM    = info:set_header(7, "Bufferram", "count", "max")
local TOTAL_SWAP    = info:set_header(8, "Totalswap", "count", "max")
local FREE_SWAP     = info:set_header(9, "Freeswap", "count", "max")
local PROCESSES     = info:set_header(10, "Processes", "count", "max")

function process_message ()
    local ts = read_message("Timestamp")

    info:set(ts, LOAD_AVG_1,    read_message("Fields[OneMinLoadAvg]"))
    info:set(ts, LOAD_AVG_5,    read_message("Fields[FiveMinLoadAvg]"))
    info:set(ts, LOAD_AVG_15,   read_message("Fields[FifteenMinLoadAvg]"))
    info:set(ts, TOTAL_RAM,     read_message("Fields[Totalram]"))
    info:set(ts, FREE_RAM,      read_message("Fields[Freeram]"))
    info:set(ts, SHARED_RAM,    read_message("Fields[Sharedram]"))
    info:set(ts, BUFFER_RAM,    read_message("Fields[Bufferram]"))
    info:set(ts, TOTAL_SWAP,    read_message("Fields[Totalswap]"))
    info:set(ts, FREE_SWAP,     read_message("Fields[Freeswap]"))
    info:set(ts, PROCESSES,     read_message("Fields[Processes]"))
    return 0
end

function timer_event(ns)
    if anomaly_config then
        if not alert.throttled(ns) then
            local msg, annos = anomaly.detect(ns, title, info, anomaly_config)
            if msg then
                annotation.concat(title, annos)
                alert.send(ns, msg)
            end
        end
        inject_payload("cbuf", title, annotation.prune(title, ns), info)
    else
        inject_payload("cbuf", title, info)
    end
end

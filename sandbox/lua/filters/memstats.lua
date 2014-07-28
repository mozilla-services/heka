-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs the memory usage of the system running heka.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60 seconds
    per row is a 24 sliding hour window with 1 minute resolution.

*Example Heka Configuration*

.. code-block:: ini

    [MemoryStatsFilter]
    type = "SandboxFilter"
    filename = "lua_filters/memstats.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Type == 'heka.stats.memstats'"

--]]

require "circular_buffer"
require "string"
require "table"
local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

local title             = "Memory Stats"
local rows              = read_config("rows") or 1440
local sec_per_row       = read_config("sec_per_row") or 60
local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))
annotation.set_prune(title, rows * sec_per_row * 1e9)

local field_names = {"MemFree", "Cached", "Active", "Inactive", "VmallocUsed", "Shmem", "SwapCached"}

-- Handle swap separately. We're going to track SwapFree, and SwapUsed
-- SwapUsed will need to be calcuated.

cbuf = circular_buffer.new(rows, #field_names+2, sec_per_row)

for i, name in pairs(field_names) do
    cbuf:set_header(i, name, "Count", "max")
end

cbuf:set_header(#field_names+1, "SwapFree", "Count", "max")
cbuf:set_header(#field_names+2, "SwapUsed", "Count", "max")


function process_message ()
    local ts = read_message("Timestamp")
    for i, name in pairs(field_names) do
        local label = string.format("Fields[%s]", name)
        cbuf:set(ts, i, read_message(label))
    end
    local swapFree = read_message("Fields[SwapFree]")
    local swapTotal = read_message("Fields[SwapTotal]")
    local swapUsed = swapTotal - swapFree

    cbuf:set(ts, #field_names+1, swapFree)
    cbuf:set(ts, #field_names+2, swapUsed)

    return 0
end

function timer_event(ns)
    if anomaly_config then
        if not alert.throttled(ns) then
            local msg, annos = anomaly.detect(ns, title, cbuf, anomaly_config)
            if msg then
                annotation.concat(title, annos)
                alert.send(ns, msg)
            end
        end
        inject_payload("cbuf", title, annotation.prune(title, ns), cbuf)
    else
        inject_payload("cbuf", title, cbuf)
    end
end
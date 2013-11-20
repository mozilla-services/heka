-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local rows = read_config("rows")
local sec_per_row = read_config("sec_per_row")
local REQUESTS    = 1
local TOTAL_SIZE  = 2

channels = {}

local function add_channel(channel)
    channels[channel] = circular_buffer.new(rows, 2, sec_per_row, true)
    local c = channels[channel]
    c:set_header(REQUESTS, "Requests") 
    c:set_header(TOTAL_SIZE, "Total Size", "KiB")
    return c
end

all = add_channel("ALL")

function process_message ()
    local ts = read_message("Timestamp")
    local rs = tonumber(read_message("Fields[size]"))
    local url = read_message("Fields[url]")

    local cnt = all:add(ts, REQUESTS, 1)
    if not cnt then return 0 end -- outside the buffer
    if rs then
        rs = rs / 1024
    else     
        rs = 0
    end
    all:add(ts, TOTAL_SIZE, rs)

    local channel = url:match("^/submit/telemetry/[^/]+/[^/]+/[^/]+/[^/]+/([^/]+)")
    if not channel then return 0 end
    if channel ~= "release" and channel ~= "beta" and channel ~= "aurora" and channel ~= "nightly" then
        channel = "other"
    end

    local c = channels[channel]
    if not c then
        c = add_channel(channel)
    end
    c:add(ts, REQUESTS, 1)
    c:add(ts, TOTAL_SIZE, rs)

    return 0
end

function timer_event(ns)
    for k, v in pairs(channels) do
        inject_message(v:format("cbuf"), k)
        inject_message(v:format("cbufd"), k)
    end
end

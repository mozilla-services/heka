-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

status = circular_buffer.new(1440, 5, 60)
local HTTP_200          = status:set_header(1, "HTTP_200"      , "count")
local HTTP_300          = status:set_header(2, "HTTP_300"      , "count")
local HTTP_400          = status:set_header(3, "HTTP_400"      , "count")
local HTTP_500          = status:set_header(4, "HTTP_500"      , "count")
local HTTP_UNKNOWN      = status:set_header(5, "HTTP_UNKNOWN"  , "count")

request = circular_buffer.new(1440, 4, 60)
local SUCCESS           = request:set_header(1, "Success"      , "count")
local FAILURE           = request:set_header(2, "Failure"      , "count")
local AVG_RESPONSE_SIZE = request:set_header(3, "Response Size", "B", "avg")
local AVG_RESPONSE_TIME = request:set_header(4, "Response Time", "s", "avg")

sums = circular_buffer.new(1440, 3, 60)
local REQUESTS      = sums:set_header(1, "Requests"      , "count")
local RESPONSE_SIZE = sums:set_header(2, "Response Size" , "B")
local RESPONSE_TIME = sums:set_header(3, "Response Time" , "s")

function process_message ()
    local ts = read_message("Timestamp")
    local sc = read_message("Fields[StatusCode]")
    local rt = read_message("Fields[ResponseTime]")
    local rs = read_message("Fields[ResponseSize]")

    local cnt = sums:add(ts, REQUESTS, 1)
    if cnt == nil then return 0 end -- outside the buffer

    local t = sums:add(ts, RESPONSE_SIZE, tonumber(rs))
    request:set(ts, AVG_RESPONSE_SIZE, t/cnt)
    t = sums:add(ts, RESPONSE_TIME, tonumber(rt))
    request:set(ts, AVG_RESPONSE_TIME, t/cnt)

    sc = tonumber(sc)
    if sc >= 200 and sc < 300 then
        status:add(ts, HTTP_200, 1)
        request:add(ts, SUCCESS, 1)
    elseif sc >= 300  and sc < 400 then
        status:add(ts, HTTP_300, 1)
        request:add(ts, SUCCESS, 1)
    elseif sc >= 400 and sc < 500 then
        status:add(ts, HTTP_400, 1)
        request:add(ts, FAILURE, 1)
    elseif sc >= 500  and sc < 600 then
        status:add(ts, HTTP_500, 1)
        request:add(ts, FAILURE, 1)
    else
        status:add(ts, HTTP_UNKNOWN, 1)
        request:add(ts, FAILURE, 1)
    end

    return 0
end

function timer_event(ns)
    -- advance the buffers so the graphs will continue to advance without new data
    -- status:add(ns, 1, 0) 
    -- request:add(ns, 1, 0)

    output(status)
    inject_message("cbuf", "Sync Proxy Response Status")

    output(request)
    inject_message("cbuf", "Sync Proxy Request Statistics")
end


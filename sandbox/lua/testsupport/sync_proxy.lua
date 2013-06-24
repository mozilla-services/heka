-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local rows = 1440
local sec_per_row = 60

status = circular_buffer.new(rows, 5, sec_per_row)
local HTTP_200          = status:set_header(1, "HTTP_200"      , "count")
local HTTP_300          = status:set_header(2, "HTTP_300"      , "count")
local HTTP_400          = status:set_header(3, "HTTP_400"      , "count")
local HTTP_500          = status:set_header(4, "HTTP_500"      , "count")
local HTTP_UNKNOWN      = status:set_header(5, "HTTP_UNKNOWN"  , "count")

request = circular_buffer.new(rows, 4, sec_per_row)
local SUCCESS           = request:set_header(1, "Success"      , "count")
local FAILURE           = request:set_header(2, "Failure"      , "count")
local AVG_RESPONSE_SIZE = request:set_header(3, "Response Size", "B", "avg")
local AVG_RESPONSE_TIME = request:set_header(4, "Response Time", "s", "avg")

sums = circular_buffer.new(rows, 3, sec_per_row)
local REQUESTS      = sums:set_header(1, "Requests"      , "count")
local RESPONSE_SIZE = sums:set_header(2, "Response Size" , "B")
local RESPONSE_TIME = sums:set_header(3, "Response Time" , "s")

local interval = 1e9 * sec_per_row
local sliding_window = interval * 15

newest = 0
oldest = 0
last_alert = 0
annotations = {_name="annotations"}
annotations_size = 0

function process_message ()
    local ts = read_message("Timestamp")
    local sc = read_message("Fields[StatusCode]")
    local rt = read_message("Fields[ResponseTime]")
    local rs = read_message("Fields[ResponseSize]")

    local cnt = sums:add(ts, REQUESTS, 1)
    if cnt == nil then return 0 end -- outside the buffer

    if ts > newest then newest = ts end
    if oldest == 0 then oldest = ts end

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
    inject_message("cbuf", "HTTP Status")

    if newest - interval - oldest < sliding_window * 2 then
        output(request)
        inject_message("cbuf", "Request Statistics")
        return -- not enough data to check for anomalies
    end

    -- Anomaly detection
    -- Compute the average of the last 15 intervals and the 15 intervals before
    -- that and compare the difference against the historical standard deviation.
    -- The current interval is not included since it is incomplete and can skew
    -- the stats.
    local previous_window = newest - sliding_window * 2
    local current_window = newest - sliding_window
    local historical_sd = request:compute("sd", AVG_RESPONSE_TIME, nil, previous_window - interval) 
    local previous_avg = request:compute("avg", AVG_RESPONSE_TIME, previous_window, current_window - interval)
    local current_avg = request:compute("avg", AVG_RESPONSE_TIME, current_window, newest - interval)

    local delta = math.abs(current_avg - previous_avg)
    if delta > historical_sd * 2 and newest - last_alert > sliding_window then
        for i=1, annotations_size do -- clean out old alerts
            if annotations[i].x < (newest - interval * rows)/1e6 then 
                table.remove(annotations, i)
                annotations_size = annotations_size - 1
            else
                break
            end 
        end

        local msg = "CRITICAL:Average response time has fluxuated more than 2 standard deviations"
        last_alert = newest - newest % interval
        annotations_size = annotations_size + 1
        annotations[annotations_size] = {x          = math.floor(last_alert/1e6),
                                         col        = AVG_RESPONSE_TIME, 
                                         shortText  = "A", 
                                         text       = msg}
        output(msg)
        inject_message("nagios-external-command", "PROCESS_SERVICE_CHECK_RESULT")
    end

    output(annotations, request)
    inject_message("cbuf", "Request Statistics")
end


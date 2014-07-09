-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs HTTP status codes using the numeric Fields[status] variable collected
from web server access logs.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60 seconds
    per row is a 24 sliding hour window with 1 minute resolution.

- anomaly_config(string) - (see :ref:`sandbox_anomaly_module`)

*Example Heka Configuration*

.. code-block:: ini

    [FxaAuthServerHTTPStatus]
    type = "SandboxFilter"
    filename = "lua_filters/http_status.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Logger == 'nginx.access' && Type == 'fxa-auth-server'"

    [FxaAuthServerHTTPStatus.config]
    sec_per_row = 60
    rows = 1440
    anomaly_config = 'roc("HTTP Status", 1, 15, 0, 1.5, true, false)'
--]]

require "circular_buffer"
require "string"
local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

local title             = "HTTP Status"
local rows              = read_config("rows") or 1440
local sec_per_row       = read_config("sec_per_row") or 60
local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))
annotation.set_prune(title, rows * sec_per_row * 1e9)

status = circular_buffer.new(rows, 5, sec_per_row)
local HTTP_200      = status:set_header(1, "HTTP_200")
local HTTP_300      = status:set_header(2, "HTTP_300")
local HTTP_400      = status:set_header(3, "HTTP_400")
local HTTP_500      = status:set_header(4, "HTTP_500")
local HTTP_UNKNOWN  = status:set_header(5, "HTTP_UNKNOWN")

function process_message ()
    local ts = read_message("Timestamp")
    local sc = read_message("Fields[status]")

    if sc >= 200 and sc < 300 then
        status:add(ts, HTTP_200, 1)
    elseif sc >= 300  and sc < 400 then
        status:add(ts, HTTP_300, 1)
    elseif sc >= 400 and sc < 500 then
        status:add(ts, HTTP_400, 1)
    elseif sc >= 500  and sc < 600 then
        status:add(ts, HTTP_500, 1)
    else
        status:add(ts, HTTP_UNKNOWN, 1)
    end

    return 0
end

function timer_event(ns)
    if anomaly_config then
        if not alert.throttled(ns) then
            local msg, annos = anomaly.detect(ns, title, status, anomaly_config)
            if msg then
                annotation.concat(title, annos)
                alert.send(ns, msg)
            end
        end
        inject_payload("cbuf", title, annotation.prune(title, ns), status)
    else
        inject_payload("cbuf", title, status)
    end
end

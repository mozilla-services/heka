-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs HTTP status codes using the numeric Fields[status] variable collected
from web server access logs.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding
    window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60
    seconds per row is a 24 sliding hour window with 1 minute resolution.

- anomaly_config (string, optional)
    See :ref:`sandbox_anomaly_module`.

- alert_throttle (uint, optional, default 3600)
    Sets the throttle for the anomaly alert, in seconds.

- preservation_version (uint, optional, default 0)
    If `preserve_data = true` is set in the SandboxFilter configuration, then
    this value should be incremented every time the `sec_per_row` or `rows`
    configuration is changed to prevent the plugin from failing to start
    during data restoration.

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
    anomaly_config = 'roc("HTTP Status", 2, 15, 0, 1.5, true, false) roc("HTTP Status", 4, 15, 0, 1.5, true, false) mww_nonparametric("HTTP Status", 5, 15, 10, 0.8)'
    alert_throttle = 300
    preservation_version = 0
--]]
_PRESERVATION_VERSION = read_config("preservation_version") or 0
_PRESERVATION_VERSION = _PRESERVATION_VERSION + 1 -- force a revision update due to internal changes

require "circular_buffer"
require "string"
local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

local title             = "HTTP Status"
local rows              = read_config("rows") or 1440
local sec_per_row       = read_config("sec_per_row") or 60
local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))
local alert_throttle    = read_config("alert_throttle") or 3600
alert.set_throttle(alert_throttle * 1e9)

annotation.set_prune(title, rows * sec_per_row * 1e9)

status = circular_buffer.new(rows, 6, sec_per_row)
status:set_header(1, "HTTP_100")
status:set_header(2, "HTTP_200")
status:set_header(3, "HTTP_300")
status:set_header(4, "HTTP_400")
status:set_header(5, "HTTP_500")
local HTTP_UNKNOWN = status:set_header(6, "HTTP_UNKNOWN")

function process_message ()
    local ts = read_message("Timestamp")
    local sc = read_message("Fields[status]")
    if type(sc) ~= "number" then return -1 end

    local col = sc/100
    if col >= 1 and col < 6 then
        status:add(ts, col, 1) -- col will be truncated to an int
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

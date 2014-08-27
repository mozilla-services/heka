-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Graphs MySQL slow query data produced by the :ref:`config_mysql_slow_query_log_decoder`.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- rows (uint, optional, default 1440)
    Sets the size of the sliding window i.e., 1440 rows representing 60 seconds
    per row is a 24 sliding hour window with 1 minute resolution.

- anomaly_config (string, optional)
    See :ref:`sandbox_anomaly_module`.

- preservation_version (uint, optional, default 0)
    If `preserve_data = true` is set in the SandboxFilter configuration, then
    this value should be incremented every time the `sec_per_row` or `rows`
    configuration is changed to prevent the plugin from failing to start
    during data restoration.

*Example Heka Configuration*

.. code-block:: ini

    [Sync-1_5-SlowQueries]
    type = "SandboxFilter"
    message_matcher = "Logger == 'Sync-1_5-SlowQuery'"
    ticker_interval = 60
    filename = "lua_filters/mysql_slow_query.lua"

        [Sync-1_5-SlowQueries.config]
        anomaly_config = 'mww_nonparametric("Statistics", 5, 15, 10, 0.8)'
        preservation_version = 0
--]]
_PRESERVATION_VERSION = read_config("preservation_version") or 0

require "circular_buffer"
require "math"
local alert         = require "alert"
local annotation    = require "annotation"
local anomaly       = require "anomaly"

local title             = "Statistics"
local rows              = read_config("rows") or 1440
local sec_per_row       = read_config("sec_per_row") or 60
local anomaly_config    = anomaly.parse_config(read_config("anomaly_config"))
annotation.set_prune(title, rows * sec_per_row * 1e9)

data = circular_buffer.new(rows, 5, sec_per_row)
sums = circular_buffer.new(rows, 4, sec_per_row)
local QUERY_TIME    = data:set_header(1, "Query Time"   , "s"       , "none")
local LOCK_TIME     = data:set_header(2, "Lock Time"    , "s"       , "none")
local ROWS_EXAMINED = data:set_header(3, "Rows Examined", "count"   , "none")
local ROWS_SENT     = data:set_header(4, "Rows Sent"    , "count"   , "none")
local COUNT         = data:set_header(5, "Count")

local floor = math.floor
function process_message ()
    local ts = read_message("Timestamp")
    local cnt = data:add(ts, COUNT, 1)
    if not cnt then return 0 end

    local qt = read_message("Fields[Query_time]")
    if type(qt) ~= "number" then return -1 end

    local lt = read_message("Fields[Lock_time]")
    if type(lt) ~= "number" then return -1 end

    local re = read_message("Fields[Rows_examined]")
    if type(re) ~= "number" then return -1 end

    local rs = read_message("Fields[Rows_sent]")
    if type(rs) ~= "number" then return -1 end

    data:set(ts, QUERY_TIME, sums:add(ts, QUERY_TIME, qt)/cnt)
    data:set(ts, LOCK_TIME, sums:add(ts, LOCK_TIME, lt)/cnt)
    data:set(ts, ROWS_EXAMINED, floor(sums:add(ts, ROWS_EXAMINED, re)/cnt))
    data:set(ts, ROWS_SENT, floor(sums:add(ts, ROWS_SENT, rs)/cnt))
    return 0
end

function timer_event(ns)
    if anomaly_config then
        if not alert.throttled(ns) then
            local msg, annos = anomaly.detect(ns, title, data, anomaly_config)
            if msg then
                annotation.concat(title, annos)
                alert.send(ns, msg)
            end
        end
        inject_payload("cbuf", title, annotation.prune(title, ns), data)
    else
        inject_payload("cbuf", title, data)
    end
end

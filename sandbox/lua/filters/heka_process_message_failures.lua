-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Monitors Heka's process message failures by plugin.

Config:

- anomaly_config(string) - (see :ref:`sandbox_anomaly_module`)
    A list of anomaly detection specifications.  If not specified a default of
    'mww_nonparametric("DEFAULT", 1, 5, 10, 0.7)' is used. The "DEFAULT"
    settings are applied to any plugin without an explict specification.

*Example Heka Configuration*

.. code-block:: ini

    [HekaProcessMessageFailures]
    type = "SandboxFilter"
    filename = "lua_filters/heka_process_message_failures.lua"
    ticker_interval = 60
    preserve_data = false # the counts are reset on Heka restarts and the monitoring should be too.
    message_matcher = "Type == 'heka.all-report'"
--]]

local alert      = require "alert"
local anomaly    = require "anomaly"
require "circular_buffer"
require "cjson"

local sections       = {"inputs", "decoders", "filters", "encoders", "outputs"}
local anomaly_config = anomaly.parse_config(read_config("anomaly_config") or 'mww_nonparametric("DEFAULT", 1, 5, 10, 0.7)')
local plugins        = {}
local last_report    = 0

local function find_plugin(name)
    local p = plugins[name]
    if not p then
        local cb = circular_buffer.new(60, 1, 60)
        cb:set_header(1, "Failures", "count", "max")
        p = {last_report = 0, last_alert = 0, cbuf = cb}
        plugins[name] = p
        if not anomaly_config[name] and anomaly_config["DEFAULT"] then
            anomaly_config[name] = anomaly_config["DEFAULT"]
        end
    end
    return p
end

function process_message ()
    local ok, json = pcall(cjson.decode, read_message("Payload"))
    if not ok then return -1 end

    last_report = read_message("Timestamp")
    for n,section in ipairs(sections) do
        local t = json[section] or {}
        for i,v in ipairs(t) do
            if type(v) ~= "table" then return -1, "invalid entry in section " .. section end
            if type(v.ProcessMessageFailures) == "table" then -- confirm this plugin has instrumentation
                if not v.Name then return -1, "missing plugin Name" end
                local p = find_plugin(v.Name)
                p.last_report = last_report
                local n = v.ProcessMessageFailures.value
                if type(n) == "number" and n > 0 then -- NaN <-> 0 transitions are considered an anomaly
                    p.cbuf:set(last_report, 1, n)
                end
            end
        end
    end

    return 0
end

function timer_event(ns)
    for k,v in pairs(plugins) do
        if ns - v.last_alert > 60 * 60 * 1e9 then -- manual throttling (one alert per plugin per hour)
            local msg, annos = anomaly.detect(ns, k, v.cbuf, anomaly_config)
            if msg then
                alert.queue(0, msg)
                v.last_alert = ns
            end
        end
        if last_report ~= v.last_report then -- prune dynamic decoders/filters
            plugins[k] = nil
            anomaly_config[k] = nil
        end
    end
    alert.send_queue(0)
end

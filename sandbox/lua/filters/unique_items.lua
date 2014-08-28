-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Counts the number of unique items per day e.g. active daily users by uid.

Config:

- message_variable (string, required)
    The Heka message variable containing the item to be counted.

- title (string, optional, default "Estimated Unique Daily *message_variable*")
    The graph title for the cbuf output.

- enable_delta (bool, optional, default false)
    Specifies whether or not this plugin should generate cbuf deltas. Deltas
    should be enabled when sharding is used;
    see: :ref:`config_circular_buffer_delta_agg_filter`.

- preservation_version (uint, optional, default 0)
    If `preserve_data = true` is set in the SandboxFilter configuration, then
    this value should be incremented every time the `enable_delta`
    configuration is changed to prevent the plugin from failing to start
    during data restoration.

*Example Heka Configuration*

.. code-block:: ini

    [FxaActiveDailyUsers]
    type = "SandboxFilter"
    filename = "lua_filters/unique_items.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Logger == 'FxaAuth' && Type == 'request.summary' && Fields[path] == '/v1/certificate/sign' && Fields[errno] == 0"

        [FxaActiveDailyUsers.config]
        message_variable = "Fields[uid]"
        title = "Estimated Active Daily Users"
        preservation_version = 0
--]]
_PRESERVATION_VERSION = read_config("preservation_version") or 0

require "hyperloglog"
require "circular_buffer"
require "math"
require "string"

local message_variable  = read_config("message_variable") or error("message_variable configuration must be specified")
local title             = read_config("title") or "Estimated Unique Daily " .. message_variable
local enable_delta      = read_config("enable_delta") or false

active_day  = 0
hll         = hyperloglog.new()
active      = circular_buffer.new(365, 1, 60 * 60 * 24, enable_delta)
local USERS = active:set_header(1, message_variable:match("^Fields%[([^%]]+)%]") or message_variable)
local floor = math.floor

function process_message ()
    local ts = read_message("Timestamp")

    local day = floor(ts / (60 * 60 * 24 * 1e9))
    if day < active_day  then
        return 0 -- too old
    elseif day > active_day then
        active_day = day
        hll:clear()
    end

    local item = read_message(message_variable)
    if not(type(item) == "string" or type(item) == "number") then return -1 end

    if hll:add(item) then
        active:set(ts, USERS, hll:count())
    end

    return 0
end

function timer_event(ns)
    inject_payload("cbuf", title, active:format("cbuf"))
    if enable_delta then
        inject_payload("cbufd", title, active:format("cbufd"))
    end
end


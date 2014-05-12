-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Counts the number of unique items per day e.g. active daily users by uid.

Config:

- message_variable (string, required)
    The Heka message variable containing the item to be counted.

- max_items (int, optional, default 1M)
    The maximum number of items that will be added to the bloom filter (must be
    greater than zero).

- probability (double, optional, default 0.01)
    The probability of false positives (must be between 0 and 1).

- title (string, optional, default "Estimated Unique Daily *message_variable*")
    The graph title for the cbuf output.

- enable_delta (bool, optional, default false)
    Specifies whether or not this plugin should generate cbuf deltas. Deltas
    should be enabled when sharding is used too accommodate very large data
    sets; see: :ref:`config_circular_buffer_delta_agg_filter`.

*Example Heka Configuration*

.. code-block:: ini

    [FxaActiveDailyUsers]
    type = "SandboxFilter"
    script_type = "lua"
    filename = "lua_filters/unique_items.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Logger == 'FxaAuth' && Type == 'request.summary' && Fields[path] == '/v1/certificate/sign' && Fields[errno] == 0"

        [FxaActiveDailyUsers.config]
        message_variable = "Fields[uid]"
        max_items = 6000000
        title = "Estimated Active Daily Users"
--]]

require "bloom_filter"
require "circular_buffer"
require "math"
require "string"

local message_variable  = read_config("message_variable") or error("message_variable is required")
local max_items         = read_config("max_items") or 1e6
local probability       = read_config("probability") or 0.01
local title             = read_config("title") or "Estimated Unique Daily " .. message_variable
local enable_delta      = read_config("enable_delta") or false

active_day  = 0
bf          = bloom_filter.new(max_items, probability)
active      = circular_buffer.new(365, 1, 60 * 60 * 24, enable_delta)
local USERS = active:set_header(1, message_variable:match("^Fields%[([^%]]+)%]") or message_variable)

function process_message ()
    local ts = read_message("Timestamp")

    local day = math.floor(ts / (60 * 60 * 24 * 1e9))
    if day < active_day  then
        return 0 -- too old
    elseif day > active_day then
        active_day = day
        bf:clear()
    end

    local item = read_message(message_variable)
    if bf:add(item) then
        active:add(ts, USERS, 1)
    end

    return 0
end

function timer_event(ns)
    inject_message(active:format("cbuf"), title)
    if enable_delta then
        inject_message(active:format("cbufd"), title)
    end
end

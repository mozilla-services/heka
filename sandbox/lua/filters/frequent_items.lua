-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Calculates the most frequent items in a data stream.

Config:

- message_variable (string)
    The message variable name containing the items to be counted.

- max_items (uint, optional, default 1000)
    The maximum size of the sample set (higher will produce a more accurate
    list).

- min_output_weight (uint, optional, default 100)
    Used to reduce the long tail output by only outputting the higher frequency
    items.

- reset_days (uint, optional, default 1)
    Resets the list after the specified number of days (on the UTC day
    boundary).  A value of 0 will never reset the list.

*Example Heka Configuration*

.. code-block:: ini

    [FxaAuthServerFrequentIP]
    type = "SandboxFilter"
    filename = "lua_filters/frequent_items.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Logger == 'nginx.access' && Type == 'fxa-auth-server'"

    [FxaAuthServerFrequentIP.config]
    message_variable = "Fields[remote_addr]"
    max_items = 10000
    min_output_weight = 100
    reset_days = 1
--]]

require "math"
require "string"

local message_variable  = read_config("message_variable") or error("message_variable configuration must be specified")
local max_items         = read_config("max_items") or 1000
local min_output_weight = read_config("min_output_weight") or 100
local reset_days        = read_config("reset_days") or 1

items       = {}
items_size  = 0
last_reset  = 0

function process_message ()
    local item = read_message(message_variable)
    if not item then return -1 end

    if reset_days > 0 then
        local ts = read_message("Timestamp")
        local day = math.floor(ts / (60 * 60 * 24 * 1e9))
        local days = day - last_reset
        if days < 0 then
            return 0 -- too old
        elseif days >= reset_days then
            last_reset = day
            items = {}
            items_size = 0
        end
    end

    local i = items[item]
    if i then
        items[item] = i + 1
        return 0
    end

    if items_size >= max_items then
        for k,v in pairs(items) do
            if v == 1 then
                items[k] = nil
                items_size = items_size - 1
            else
                items[k] = v - 1
            end
        end
    else
        items[item] = 1
        items_size = items_size + 1
    end

    return 0
end

function timer_event(ns)
    add_to_payload(message_variable, "\tWeight\n")
    for k, v in pairs(items) do
        if v > min_output_weight then
            add_to_payload(string.format("%s\t%d\n", k, v))
        end
    end
    inject_payload("tsv", string.format("Weighting by %s", message_variable))
end

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
    script_type = "lua"
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
require "os"
require "string"

local message_variable = read_config("message_variable")
local max_items = read_config("max_items") or 1000
local min_output_weight = read_config("min_output_weight") or 100
local reset_days = read_config("reset_days") or 1

local WEIGHT = 1

local function get_day_number()
    return math.floor(os.time() / (60 * 60 * 24))
end

items = {}
items_size = 0
day = get_day_number()

function process_message ()
    local item = read_message(message_variable)
    if item == nil then return 0 end

    local i = items[item]
    if i == nil  then
        if items_size == max_items then
            for k,v in pairs(items) do
                v[WEIGHT] = v[WEIGHT] - 1
                if  v[WEIGHT] == 0 then
                    items[k] = nil
                    items_size = items_size - 1
                end
            end
        else
            i = {0}
            items[item] = i
            items_size = items_size + 1
        end
    end

    if i ~= nil then
        i[WEIGHT] = i[WEIGHT] + 1
    end

    return 0
end

function timer_event(ns)
    output(message_variable, "\tWeight\n")
    for k, v in pairs(items) do
        if v[WEIGHT] > min_output_weight then
            output(string.format("%s\t%d\n", k, v[WEIGHT]))
        end
    end
    inject_message("tsv", string.format("Weighting by %s", message_variable))

    if reset_days > 0 then
        local current_day = get_day_number()
        if current_day - day >= reset_days then
            day = current_day
            items = {}
            items_size = 0
        end
    end
end


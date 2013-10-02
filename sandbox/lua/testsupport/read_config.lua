-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local s = read_config("string")
if s ~= "widget" then error("string") end

local n = read_config("int64")
if n ~= 99 then return error("int") end

local d = read_config("double")
if d ~= 99.123 then error("double") end

local b = read_config("bool")
if b ~= true then error("bool") end

local n = read_config("nil")
if n ~= nil then error("nil") end

local a = read_config("array")
if a ~= nil then error("array") end

local o = read_config("object")
if o ~= nil then return error("object") end

function process_message ()
    return 0
end

function timer_event()
end


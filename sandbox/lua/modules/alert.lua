-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
---
**queue(ns, msg)**
    Queue an alert message to be sent.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.
        - msg (string) alert payload.

    *Return*
        - true if the message is queued, false if it would be throttled.


**send(ns, msg)**
    Send and alert message.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.
        - msg (string) alert payload.

    *Return*
        - true if the message is sent, false if it is throttled.

**send_queue(ns)**
    Sends all queued alert message as a single message.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.

    *Return*
        - true if the queued messages are sent, false if they are throttled.

**set_throttle(ns_duration)**
    Sets the minimum duration between alert event outputs.

    *Arguments*
        - ns_duration (int64) minimum duration in nanoseconds between alerts.

    *Return*
        - none

**throttled(ns)**
    Test to see if sending an alert at this time would be throttled.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.

    *Return*
        - true if a message would be throttled, false if it would be sent.

.. note::

    Use a zero timestamp to override message throttling.
--]]

-- Imports
local table = require "table"
local output = output
local inject_message = inject_message

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module

local alerts        = nil
local last_alert    = 0
local throttle      = 60 * 60 * 1e9 -- maximum 1 message per hour

function queue(ns, msg)
    if not msg or msg == "" or throttled(ns) then
        return false
    end

    if not alerts then
        alerts = {}
    end
    table.insert(alerts, msg)

    return true
end


function send(ns, msg)
    if not msg or msg == "" or throttled(ns) then
        return false
    end
    output(msg)
    inject_message("alert")

    if ns > last_alert then
        last_alert = ns
    end

    return true
end


function send_queue(ns)
    if alerts == nil then
        return false
    end

    local msg = table.concat(alerts, "\n")
    alerts = nil

    return send(ns, msg)
end


function set_throttle(ns_duration)
    throttle = ns_duration
end


function throttled(ns)
    if ns == 0 then
        return false
    elseif ns < last_alert or ns - last_alert <= throttle then
        return true
    end
    return false
end

return M

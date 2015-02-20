-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
^^^
Stores the last alert time in the global *_LAST_ALERT* so alert throttling will
persist between restarts.

**queue(ns, msg)**
    Queue an alert message to be sent.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.
        - msg (string) alert payload.

    *Return*
        - true if the message is queued, false if it would be throttled.


**send(ns, msg)**
    Send an alert message.

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


-- Global Exports
_LAST_ALERT = 0 -- throw the time into global space so it is preserved. Not
                -- really liking this but from a usability and preservation
                -- perspective it makes things more seamless.

-- Imports
require "table"

local M = {}
-- We cannot remove external access because we need to use the global
-- _LAST_ALERT and have access to the value after data restoration.

local alerts        = nil
local throttle      = 60 * 60 * 1e9 -- maximum 1 message per hour

function M.queue(ns, msg)
    if not msg or msg == "" or M.throttled(ns) then
        return false
    end

    if not alerts then
        alerts = {}
    end
    table.insert(alerts, msg)

    return true
end


function M.send(ns, msg)
    if not msg or msg == "" or M.throttled(ns) then
        return false
    end
    inject_payload("alert", "", msg)

    if ns > _LAST_ALERT then
        _LAST_ALERT = ns
    end

    return true
end


function M.send_queue(ns)
    if alerts == nil then
        return false
    end

    local msg = table.concat(alerts, "\n")
    alerts = nil

    return M.send(ns, msg)
end


function M.set_throttle(ns_duration)
    throttle = ns_duration
end


function M.throttled(ns)
    if ns == 0 then
        return false
    elseif ns < _LAST_ALERT or ns - _LAST_ALERT <= throttle then
        return true
    end
    return false
end

return M

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local state = 0

function process_message ()
    return 0
end


function timer_event(ns)

    if state == 0 then
        add_to_payload("OK:Ok alerts are working!")
    elseif state == 1 then
        add_to_payload("WARNING:Warning alerts are working!")
    elseif state == 2 then
        add_to_payload("CRITICAL:Critical alerts are working!")
    elseif state == 3 then
        add_to_payload("UNKNOWN:Unknown alerts are working!")
    end
    state = state + 1
    if state == 4 then state = 0 end

    inject_payload("nagios-external-command", "PROCESS_SERVICE_CHECK_RESULT")
end


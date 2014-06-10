-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "circular_buffer"

last_inject = 0
count = 0
cnts = circular_buffer.new(3600, 1, 1) -- 1 hour window with 1 second resolution
local MESSAGES = cnts:set_header(1, "Messages", "count")

function process_message ()
    -- normally we would use the message time stamp but flood messages have a fixed time
    count = count + 1
    return 0
end


function timer_event(ns)
    cnts:add(ns, MESSAGES, count)
    count = 0
    if ns - last_inject > 60e9 then -- write the aggregate once a minute
        inject_payload("cbuf", "", cnts)
        last_inject = ns
    end
end


-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.


function process_message ()
    return 0
end


function timer_event(ns)
    for i = 1, 11 do
        inject_payload("txt", "", "test")
    end
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

function process_message ()
    for i=1,200000 do
        read_message("Payload")
    end
    return 0
end

function timer_event(ns)
end


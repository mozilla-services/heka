-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local alert = require "alert"

function process_message ()
    local test = read_message("Timestamp")
    local ns = test + 60*61*1e9

    if test == 0 then
        assert(alert.send(ns, nil) == false)
        assert(alert.send(ns, "") == false)
        assert(alert.send_queue(ns) == false)
        assert(alert.queue(ns, "alert1"))
        assert(alert.queue(ns, "alert2"))
        assert(alert.queue(ns, "alert3"))
        assert(alert.send_queue(ns))
        assert(alert.send_queue(ns * 2) == false)
    elseif test == 1 then
        assert(alert.throttled(ns))
        assert(alert.send(ns, "alert4") == false)
        alert.set_throttle(10)
        ns = ns + 11
        assert(alert.send(ns, "alert5"))
        assert(alert.send(ns, "alert6") == false)
    elseif test == 2 then
        assert(alert.send(ns, "alert7") == false)
        assert(alert.send(0, "alert8"))
    end
    return 0
end

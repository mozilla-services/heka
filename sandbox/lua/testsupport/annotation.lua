-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local annotation = require "annotation"

function process_message ()
    local test = read_message("Timestamp")

    if test == 0 then
        local current_ns = 60*1e9
        annotation.add("test", 1e9, 1, "A", "anomaly")
        annotation.add("test", 5e9, 2, "A", "anomaly2")
        annotation.add("test", current_ns, 1, "M", "maintenance")
        inject_payload("json", "annotations", annotation.prune("test", current_ns))
    elseif test == 1 then
        annotation.set_prune("test", 30*1e9)
        inject_payload("json", "annotations", annotation.prune("test", 80*1e9))
    elseif test == 2 then
        inject_payload("json", "annotations", annotation.prune("test", 90*1e9))
    elseif test == 3 then
        local annos = {}
        table.insert(annos, annotation.create(5*1e9, 2, "A", "anomaly3"))
        table.insert(annos, annotation.create(90*1e9, 2, "A", "anomaly4"))
        annotation.concat("test", annos)
        assert(#_ANNOTATIONS.test.annotations == 2, #_ANNOTATIONS.test.annotations)
        local a = annotation.prune("test", 90*1e9)
        assert(#_ANNOTATIONS.test.annotations == 1, #_ANNOTATIONS.test.annotations)
        inject_payload("txt", "", "ok")
    end
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

data = circular_buffer.new(3, 3, 1, true)
local ADD_COL = data:set_header(1, "Add column")
local SET_COL = data:set_header(2, "Set column", "count")
local GET_COL = data:set_header(3, "Get column", "count", "sum")

function process_message()
    local ts = read_message("Timestamp")
    if data:add(ts, ADD_COL, 1) then
        data:set(ts, GET_COL, data:get(ts, ADD_COL))
    end
    data:set(ts, SET_COL, 1)
    return 0
end

function timer_event(ns)
    if ns == 0 then
        inject_message(data:format("cbuf"), "Method tests")
        inject_message(data:format("cbufd"), "Method tests")
    end
end

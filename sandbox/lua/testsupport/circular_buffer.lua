-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

data = circular_buffer.new(3, 3, 1)
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
        output(data)
        inject_message("cbuf", "Method tests")
    elseif ns == 1 then
        cbufs = {}
        for i=1,3,1 do
            cbufs[i] = circular_buffer.new(2,1,1)
            cbufs[i]:set_header(1, "Header_1", "count")
        end
    elseif ns == 2 then
        output(cbufs[1])
        inject_message("cbuf")
    elseif ns == 3 then
        local stats = circular_buffer.new(5, 1, 1)
        stats:set(1e9, 1, 1)
        stats:set(2e9, 1, 2)
        stats:set(3e9, 1, 3)
        stats:set(4e9, 1, 4)
        local t = stats:compute("sum", 1)
        if 10 ~= t then
            error(string.format("no range sum = %G", t))
        end
        t = stats:compute("avg", 1)
        if 2 ~= t then
            error(string.format("no range avg = %G", t))
        end
        t = stats:compute("sd", 1)
        if math.sqrt(2) ~= t then
            error(string.format("no range sd = %G", t))
        end
        t = stats:compute("min", 1)
        if 0 ~= t then
            error(string.format("no range min = %G", t))
        end
        t = stats:compute("max", 1)
        if 4 ~= t then
            error(string.format("no range max = %G", t))
        end

        t = stats:compute("sum", 1, 3e9, 4e9)
        if 7 ~= t then
            error(string.format("range 3-4 sum = %G", t))
        end
        t = stats:compute("avg", 1, 3e9, 4e9)
        if 3.5 ~= t then
            error(string.format("range 3-4 avg = %G", t))
        end
        t = stats:compute("sd", 1, 3e9, 4e9)
        if math.sqrt(0.25) ~= t then
            error(string.format("range 3-4 sd = %G", t))
        end

        t = stats:compute("sum", 1, 3e9)
        if 7 ~= t then
            error(string.format("range 3- = %G", t))
        end
        t = stats:compute("sum", 1, 3e9, nil)
        if 7 ~= t then
            error(string.format("range 3-nil = %G", t))
        end
        t = stats:compute("sum", 1, nil, 2e9)
        if 3 ~= t then
            error(string.format("range nil-2 sum = %G", t))
        end
        t = stats:compute("sum", 1, 11e9, 14e9)
        if nil ~= t then
            error(string.format("out of range = %G", t))
        end
    end
end

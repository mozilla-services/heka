-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

function process_message ()
    local msg = read_message("Payload")

    if msg == "new() incorrect # args" then
        local cb = circular_buffer.new(2)
    elseif msg == "new() non numeric row" then
        local cb = circular_buffer.new(nil, 1, 1)
    elseif msg == "new() 1 row" then
        local cb = circular_buffer.new(1, 1, 1)
    elseif msg == "new() non numeric column" then
        local cb = circular_buffer.new(2, nil, 1)
    elseif msg == "new() zero column" then
        local cb = circular_buffer.new(2, 0, 1)
    elseif msg == "new() non numeric seconds_per_row" then
        local cb = circular_buffer.new(2, 1, nil)
    elseif msg == "new() zero seconds_per_row" then
        local cb = circular_buffer.new(2, 1, 0)
    elseif msg == "new() > hour seconds_per_row" then
        local cb = circular_buffer.new(2, 1, 3601)
    elseif msg == "new() too much memory" then
        local cb = circular_buffer.new(1000, 10, 1)
    elseif msg == "set() out of range column" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(0, 2, 1.0)
    elseif msg == "set() zero column" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(0, 0, 1.0)
    elseif msg == "set() non numeric column" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(0, nil, 1.0)
    elseif msg == "set() non numeric time" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(nil, 1, 1.0)
    elseif msg == "get() invalid object" then
        local cb = circular_buffer.new(2, 1, 1)
        local invalid = 1
        cb.get(invalid, 1, 1)
    elseif msg == "set() non numeric value" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(0, 1, nil)
    elseif msg == "set() incorrect # args" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:set(0)
    elseif msg == "add() incorrect # args" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:add(0)
    elseif msg == "get() incorrect # args" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:get(0)
    elseif msg == "compute() incorrect # args" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:compute(0)
    elseif msg == "compute() incorrect function" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:compute("func", 1)
    elseif msg == "compute() incorrect column" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:compute("sum", 0)
    elseif msg == "compute() start > end" then
        local cb = circular_buffer.new(2, 1, 1)
        cb:compute("sum", 1, 2e9, 1e9)
    end
return 0
end

function timer_event()
end

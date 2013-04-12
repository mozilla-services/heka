-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

data = ""

function process_message ()
    local msg = read_message("Payload")

    if msg == "inject_message() incorrect number of args" then
        inject_message(1, 2)
    elseif msg == "output() no arg" then
        output()
    elseif msg == "out of memory" then
        for i=1,500 do
            data = data .. "012345678901234567890123456789010123456789012345678901234567890123456789012345678901234567890123456789"
        end
    elseif msg == "out of instructions" then
        while true do
        end
    elseif msg == "operation on a nil" then
        x = x + 1
    elseif msg == "invalid return" then
        return nil
    elseif msg == "no return" then
        return
    elseif msg == "read_message() incorrect number of args" then
        read_message("Type", 1, 1, 1)
    elseif msg == "read_message() incorrect field name type" then
        read_message(nil)
    elseif msg == "read_message() incorrect field index type" then
        read_message("Type", nil)
    elseif msg == "read_message() incorrect array index type" then
        read_message("Type", 0, nil)
    elseif msg == "read_message() negative field index" then
        read_message("Type", -1, 0)
    elseif msg == "read_message() negative array index" then
        read_message("Type", 0, -1)
    elseif msg == "output limit exceeded" then
        for i=1,10 do
            output("012345678901234567890123456789010123456789012345678901234567890123456789012345678901234567890123456789")
        end
    end
    return 0
end

function timer_event()
end

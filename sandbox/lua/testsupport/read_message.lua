-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "string"

data = ""

function process_message ()
    local tmp = read_message("Hostname")
    if string.len(tmp) == 0 then return 1 end

    local uuid = read_message("Uuid")
    if string.len(tmp) == 0 then return 2 end

    if read_message("Payload") ~= "" then return 3 end
    if read_message("Logger") ~= "GoSpec" then return 4 end
    if read_message("EnvVersion") ~= "0.8" then return 5 end
    if read_message("Fields[foo]") ~= "bar" then return 6 end
    if read_message("Fields[foo]", 0) ~= "bar" then return 7 end
    if read_message("Fields[foo]", 0, 0) ~= "bar" then return 8 end
    if read_message("Fields[foo]", 1) ~= "alternate" then return 9 end
    if read_message("Fields[foo]", 1, 1) ~= nil then return 10 end
    if read_message("Fields[bytes]") ~= "data" then return 11 end
    if read_message("Bogus") ~= nil then return 12 end
    if read_message("Timestamp") ~= 5123456789 then return 13 end
    if read_message("Severity") ~= 6 then return 14 end
    if read_message("Pid") == 9283 then return 15 end
    if read_message("Fields[bool]") ~= true then return 16 end
    if read_message("Fields[int]") ~= 999 then return 17 end
    if read_message("Fields[double]") ~= 99.9 then return 18 end
    if read_message("Type") ~= "TEST" then return 19 end
    if read_message("raw") ~= "rawdata" then return 20 end
    if read_message("Fields[empty_bytes]") ~= nil then return 21 end

    return 0
end


function timer_event()
    if read_message("Payload") ~= nil then
        x = x + 1 -- create a runtime error
    end
end

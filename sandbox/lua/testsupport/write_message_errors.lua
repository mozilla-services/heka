-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.



function process_message ()
    local msg = read_message("Payload")

    if msg == "too few parameters" then
        write_message()
    elseif msg == "too many parameters" then
        write_message("Fields[bogus]", 0, "count", 0, 0, 0)
    elseif msg == "Unknown field name" then
        write_message("Unknown", 0)
    elseif msg == "Missing fields specifier" then
        write_message("[Bogus]", 0)
    elseif msg == "Missing closing bracket" then
        write_message("Fields[Bogus", 0)
    elseif msg == "Out of range field index" then
        write_message("Fields[bogus]", 0, "count", 200, 0)
    elseif msg == "Negative field index" then
        write_message("Fields[bogus]", 0, "count", -1, 0)
    elseif msg == "Negative array index" then
        write_message("Fields[bogus]", 0, "count", 0, -1)
    elseif msg == "nil field" then
        write_message(nil, 0)
    elseif msg == "empty uuid" then
        write_message("Uuid", "")
    elseif msg == "invalid uuid" then
        write_message("Uuid", "abcdefghijklmnopqrst")
    elseif msg == "empty timestamp" then
        write_message("Timestamp", "")
    elseif msg == "invalid timestamp" then
        write_message("Timestamp", "invalid")
    elseif msg == "bool severity" then
        write_message("Severity", true)
    elseif msg == "double hostname" then
        write_message("Hostname", 99)
    elseif msg == "invalid field type" then
        write_message("Fields[bogus]", {a = 1})
    elseif msg == "out of range field index deletion" then
        write_message("Fields[foo]", nil, "", 2)
    elseif msg == "out of range field array index deletion" then
        write_message("Fields[int]", nil, "", 0, 2)
    end

    return 0
end

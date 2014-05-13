-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local msg = {
    Timestamp = nil,
    Hostname = nil,
    Payload = "mutated",
    Type = "after",
    Pid = nil,
    Severity = nil,
    Fields = nil
}

function process_message()
    msg.Timestamp = read_message("Timestamp")
    msg.Hostname = read_message("Hostname")
    msg.Pid = read_message("Pid")
    msg.Severity = read_message("Severity")
    inject_message(msg)
    return 0
end

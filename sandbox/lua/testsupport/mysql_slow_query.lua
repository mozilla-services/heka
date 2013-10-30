-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

data = circular_buffer.new(1440, 4, 60)
sums = circular_buffer.new(1440, 3, 60)
local QUERY_TIME    = data:set_header(1, "Query Time", "s", "avg")
local LOCK_TIME     = data:set_header(2, "Lock Time", "s", "avg")
local RESPONSE_SIZE = data:set_header(3, "Response Size", "B", "avg")
local COUNT         = data:set_header(4, "Count")

function process_message ()
    local ns = read_message("Timestamp")
    local cnt = data:add(ns, COUNT, 1)
    if not cnt then return 0 end

    local qt = read_message("Fields[Query_time]")
    local lt = read_message("Fields[Lock_time]")
    local bs = read_message("Fields[Bytes_sent]")
    data:set(ns, QUERY_TIME, sums:add(ns, QUERY_TIME, qt)/cnt)
    data:set(ns, LOCK_TIME, sums:add(ns, LOCK_TIME, lt)/cnt)
    data:set(ns, RESPONSE_SIZE, sums:add(ns, RESPONSE_SIZE, bs)/cnt)
    return 0
end

function timer_event(ns)
    output(data)
    inject_message("cbuf", "Statistics")
end

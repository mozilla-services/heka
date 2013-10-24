-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

-- sample mysql slow query log entry 
-- # User@Host: weaverw[weaverw] @  [10.14.214.13]
-- # Thread_id: 78959  Schema: weave3  Last_errno: 0  Killed: 0
-- # Query_time: 10.749944  Lock_time: 0.017599  Rows_sent: 0  Rows_examined: 0  Rows_affected: 10  Rows_read: 12
-- # Bytes_sent: 51  Tmp_tables: 0  Tmp_disk_tables: 0  Tmp_table_sizes: 0
-- # InnoDB_trx_id: 98CF
-- use weave3;
-- SET timestamp=1364506803;
-- <query>

data = circular_buffer.new(1440, 4, 60)
sums = circular_buffer.new(1440, 3, 60)
local QUERY_TIME    = data:set_header(1, "Query Time", "s", "avg")
local LOCK_TIME     = data:set_header(2, "Lock Time", "s", "avg")
local RESPONSE_SIZE = data:set_header(3, "Response Size", "B", "avg")
local COUNT         = data:set_header(4, "Count")

entry = {}
active_pattern = 1

function process_message ()
    local payload = read_message("Payload")
    if active_pattern == 1 then
        if payload:find("^#%s+User@Host:") then
            if entry.timestamp and entry.timestamp ~= 0 then
                local ns = entry.timestamp * 1e9
                local cnt = data:add(ns, COUNT, 1)
                if cnt then
                    data:set(ns, QUERY_TIME, sums:add(ns, QUERY_TIME, entry.query_time)/cnt)
                    data:set(ns, LOCK_TIME, sums:add(ns, LOCK_TIME, entry.lock_time)/cnt)
                    data:set(ns, RESPONSE_SIZE, sums:add(ns, RESPONSE_SIZE, entry.bytes_sent)/cnt)
                end
            end
            entry.query_time = 0
            entry.lock_time = 0
            entry.bytes_sent = 0
            entry.timestamp = 0
            active_pattern = 2
        end
    elseif active_pattern == 2 then
        local query_time, lock_time = payload:match("^#%s+Query_time:%s+(%d+%.%d+)%s+Lock_time:%s+(%d+%.%d+)")
        if query_time then
            entry.query_time = tonumber(query_time)
            entry.lock_time = tonumber(lock_time)
            active_pattern = 3
        end
    elseif active_pattern == 3 then
        local bytes_sent = payload:match("^#%s+Bytes_sent:%s+(%d+)")
        if bytes_sent then
            entry.bytes_sent = tonumber(bytes_sent)
            active_pattern = 4
        end
    elseif active_pattern == 4 then
        local timestamp = payload:match("^SET%s+timestamp=(%d+);")
        if timestamp then
            entry.timestamp = tonumber(timestamp)
            active_pattern = 1
        end
    end
    return 0
end

function timer_event(ns)
    inject_message(data, "Statistics")
end

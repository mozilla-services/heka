-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

-- sample mysql slow query log entry
------------------------------------
-- # User@Host: weaverw[weaverw] @  [10.14.214.13]
-- # Thread_id: 78959  Schema: weave3  Last_errno: 0  Killed: 0
-- # Query_time: 10.749944  Lock_time: 0.017599  Rows_sent: 0  Rows_examined: 0  Rows_affected: 10  Rows_read: 12
-- # Bytes_sent: 51  Tmp_tables: 0  Tmp_disk_tables: 0  Tmp_table_sizes: 0
-- # InnoDB_trx_id: 98CF
-- use weave3;
-- SET timestamp=1364506803;
-- <query>

-- Heka message produced by the grammar
---------------------------------------
-- Timestamp=1364506803000000000 (string)
-- Hostname=10.14.214.13 (string)
-- Payload=<query> (string)
-- Fields
--     Rows_read=12 (number)
--     Tmp_disk_tables=0 (number)
--     Bytes_sent=51 (number)
--     Tmp_table_sizes=0 (number)
--     Query_time=10.749944 (number)
--     Rows_examined=0 (number)
--     Tmp_tables=0 (number)
--     Lock_time=0.017599 (number)
--     Rows_sent=0 (number)
--     Schema=weave3 (string)
--     Rows_affected=10 (number)

require("lpeg")
local l = lpeg
l.locale(l)

local space = l.space^1
local sep = l.P("\n")
local line = (l.P(1) - sep)^0 * sep
local float = l.digit^1 * "." * l.digit^1

local ip_address = l.digit^-3 * "." * l.digit^-3 * "." * l.digit^-3 * "." * l.digit^-3
local user_name = l.alpha^1 * "[" * l.alpha^1 * "]"
local host_name = l.alpha^0 * l.space^0 * "[" * l.Cg(ip_address^0, "Hostname") * "]"
local user = l.P"# User@Host: " * user_name * space * "@" * space * host_name * sep

local thread_id = l.P"# Thread_id: " * l.digit^1
local schema = l.P"Schema: " * l.Cg(l.alnum^0, "Schema")
local last_errno = l.P"Last_errno: " * l.digit^1
local killed = l.P"Killed: " * l.digit^1
local thread = thread_id * space * schema * space * l.Cg(last_errno / tonumber, "Last_errno") * space * killed * sep

local query_time = l.P"# Query_time: " * l.Cg(float / tonumber, "Query_time")
local lock_time = l.P"Lock_time: " * l.Cg(float / tonumber, "Lock_time")
local rows_sent = l.P"Rows_sent: " * l.Cg(l.digit^1 / tonumber, "Rows_sent")
local rows_examined = l.P"Rows_examined: " * l.Cg(l.digit^1 / tonumber, "Rows_examined")
local rows_affected = l.P"Rows_affected: " * l.Cg(l.digit^1 / tonumber, "Rows_affected")
local rows_read = l.P"Rows_read: " * l.Cg(l.digit^1 / tonumber, "Rows_read")
local query = query_time * space * lock_time * space * rows_sent * space * rows_examined * space * rows_affected * space * rows_read * sep

local bytes_sent = l.P"# Bytes_sent: " * l.Cg(l.digit^1 / tonumber, "Bytes_sent")
local tmp_tables = l.P"Tmp_tables: " * l.Cg(l.digit^1 / tonumber, "Tmp_tables")
local tmp_disk_tables = l.P"Tmp_disk_tables: " * l.Cg(l.digit^1 / tonumber, "Tmp_disk_tables")
local tmp_table_sizes = l.P"Tmp_table_sizes: " * l.Cg(l.digit^1 / tonumber, "Tmp_table_sizes")
local bytes =  bytes_sent * space * tmp_tables * space * tmp_disk_tables * space * tmp_table_sizes * sep

local inno_db = l.P"# InnoDB" * line

local use_db = l.P"use " * line

local timestamp = l.P"SET timestamp=" * l.Cg((l.digit^1 / "%0000000000"), "Timestamp") * line

local sql = l.Cg((l.P(1))^0, "Payload")

local grammar = l.Ct(user * l.Cg(l.Ct(thread * query * bytes), "Fields") * inno_db^0 * use_db^0 * timestamp * sql)

function process_message ()
    local payload = read_message("Payload")
    local t = grammar:match(payload)
    if t then
        inject_message(t)
        return 0
    end
    return -1
end



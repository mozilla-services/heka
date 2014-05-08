-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses and transforms the MySOL slow query logs.

Config:

- env_version (string, optional, default "5_5"):
    Loads the correct MySQL slow query grammar and sets the message EnvVersion
    accordingly. Valid values: "5_1", "5_5"

- truncate_sql (int, optional, default nil)
    Truncates the SQL payload to the specified number of bytes (not UTF-8 aware)
    and appends "...". If the value is nil no truncation is performed. A
    negative value will truncate the specified number of bytes from the end.

*Example Heka Configuration*

.. code-block:: ini

    [Sync-1_5-SlowQuery]
    type = "LogstreamerInput"
    log_directory = "/var/log/mysql"
    file_match = 'mysql-slow\.log'
    parser_type = "regexp"
    delimiter = "\n(# Time:)"
    delimiter_location = "start"
    decoder = "MySql-5_5-SlowQueryDecoder"

    [MySql-5_5-SlowQueryDecoder]
    type = "SandboxDecoder"
    script_type = "lua"
    filename = "lua_decoders/mysql_slow_query.lua"

        [MySql-5_5-SlowQueryDecoder.config]
        env_version = "5_5"
        truncate_sql = 64

*Example Heka Message*

:Timestamp: 2014-05-07 15:51:28 -0700 PDT
:Type: mysql.slow-query
:Hostname: 127.0.0.1
:Pid: 0
:UUID: 5324dd93-47df-485b-a88e-429f0fcd57d6
:Logger: Sync-1_5-SlowQuery
:Payload: /* [queryName=FIND_ITEMS] */ SELECT bso.userid, bso.collection, ...
:EnvVersion: 5_5
:Severity: 7
:Fields:
    | name:"Rows_examined" value_type:DOUBLE value_double:16458
    | name:"Query_time" value_type:DOUBLE representation:"s" value_double:7.24966
    | name:"Rows_sent" value_type:DOUBLE value_double:5001
    | name:"Lock_time" value_type:DOUBLE representation:"s" value_double:0.047038

:Timestamp: 2013-03-28 14:41:03 -0700 PDT
:Type: mysql.slow-query
:Hostname: 10.14.214.12
:Pid: 0
:UUID: 316bc355-e7c2-4d02-a4ad-91ed03662e6f
:Logger: Sync-1_1-SlowQuery
:Payload: SELECT collection, MAX(modified) FROM wbo96 WHERE username='883496' GROUP BY username, collection;
:EnvVersion: 5_1
:Severity: 7
:Fields:
    | name:"Rows_read" value_type:DOUBLE value_double:0
    | name:"Tmp_disk_tables" value_type:DOUBLE value_double:0
    | name:"Bytes_sent" value_type:DOUBLE representation:"B" value_double:124
    | name:"Tmp_table_sizes" value_type:DOUBLE value_double:0
    | name:"Query_time" value_type:DOUBLE representation:"s" value_double:20.518843
    | name:"Rows_examined" value_type:DOUBLE value_double:0
    | name:"Tmp_tables" value_type:DOUBLE value_double:1
    | name:"Lock_time" value_type:DOUBLE representation:"s" value_double:0.000131
    | name:"Rows_sent" value_type:DOUBLE value_double:0
    | name:"Schema" value_string:"weave7"
    | name:"Rows_affected" value_type:DOUBLE value_double:0
--]]

require "string"
local mysql = require "mysql"

local env_version = read_config("env_version") or "5_5"
local truncate_sql = read_config("truncate_sql")
local grammar
if env_version == "5_1" then
    grammar = mysql.slow_query_grammar_5_1
elseif env_version == "5_5" then
    grammar = mysql.slow_query_grammar_5_5
else
    error("Unsupported env_version: " .. env_version)
end

function process_message ()
    local log = read_message("Payload")
    local msg = grammar:match(log)
    if not msg then return -1 end

    msg.Type = "mysql.slow-query"
    msg.EnvVersion = env_version
    if truncate_sql and #msg.Payload > truncate_sql then
        msg.Payload = string.format("%s...", msg.Payload:sub(1, truncate_sql))
    end

    inject_message(msg)
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses and transforms the MySQL slow query logs. Use mariadb_slow_query.lua to
parse the MariaDB variant of the MySQL slow query logs.

Config:

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
    delimiter = "\n(# User@Host:)"
    delimiter_location = "start"
    decoder = "MySqlSlowQueryDecoder"

    [MySqlSlowQueryDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/mysql_slow_query.lua"

        [MySqlSlowQueryDecoder.config]
        truncate_sql = 64

*Example Heka Message*

:Timestamp: 2014-05-07 15:51:28 -0700 PDT
:Type: mysql.slow-query
:Hostname: 127.0.0.1
:Pid: 0
:UUID: 5324dd93-47df-485b-a88e-429f0fcd57d6
:Logger: Sync-1_5-SlowQuery
:Payload: /\* [queryName=FIND_ITEMS] \*/ SELECT bso.userid, bso.collection, ...
:EnvVersion:
:Severity: 7
:Fields:
    | name:"Rows_examined" value_type:DOUBLE value_double:16458
    | name:"Query_time" value_type:DOUBLE representation:"s" value_double:7.24966
    | name:"Rows_sent" value_type:DOUBLE value_double:5001
    | name:"Lock_time" value_type:DOUBLE representation:"s" value_double:0.047038
--]]

require "string"
local mysql = require "mysql"

local truncate_sql = read_config("truncate_sql")

function process_message ()
    local log = read_message("Payload")
    local msg = mysql.slow_query_grammar:match(log)
    if not msg then return -1 end

    msg.Type = "mysql.slow-query"
    if truncate_sql and #msg.Payload > truncate_sql then
        msg.Payload = string.format("%s...", msg.Payload:sub(1, truncate_sql))
    end

    inject_message(msg)
    return 0
end

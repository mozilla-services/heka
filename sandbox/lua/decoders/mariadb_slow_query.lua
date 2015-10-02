-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses and transforms the MariaDB variant of the MySQL slow query logs.

Config:

- truncate_sql (int, optional, default nil)
    Truncates the SQL payload to the specified number of bytes (not UTF-8 aware)
    and appends "...". If the value is nil no truncation is performed. A
    negative value will truncate the specified number of bytes from the end.

*Example Heka Configuration*

.. code-block:: ini

    [MariadbSlowQuery]
    type = "LogstreamerInput"
    log_directory = "/var/log/mariadb"
    file_match = 'mariadb-slow\.log'
    splitter = "MariaDbSlowQuerySplitter"
    decoder = "MariaDbSlowQueryDecoder"

    [MariaDbSlowQuerySplitter]
    type = "RegexSplitter"
    delimiter = '\n(# Time: )'
    delimiter_eol = false

    [MariaDbSlowQueryDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/mariadb_slow_query.lua"

        [MariaDbSlowQueryDecoder.config]
        truncate_sql = 64

*Example Heka Message*

:Timestamp: 2014-05-07 15:51:28 -0700 PDT
:Type: mariadb.slow-query
:Hostname: test.example.com
:Pid: 0
:Uuid: 5324dd93-47df-485b-a88e-429f0fcd57d6
:Logger: MariaDbSlowQuery
:Payload: /* [queryName=FIND_ITEMS] */ SELECT bso.userid, bso.collection, ...
:EnvVersion:
:Severity: 7
:Fields:
    | name:"Rows_examined" value_type:DOUBLE value_double:16458
    | name:"Query_time" value_type:DOUBLE representation:"s" value_double:7.24966
    | name:"Rows_sent" value_type:DOUBLE value_double:5001
    | name:"Lock_time" value_type:DOUBLE representation:"s" value_double:0.047038
    | name:"QC_hit" value_type:STRING value_string:"No"
    | name:"Thread_id" value_type:DOUBLE value_double:110804
    | name:"Schema" value_type:STRING value_string:"weave0"

--]]

require "string"
local mysql = require "mysql"

local truncate_sql = read_config("truncate_sql")

function process_message ()
    local log = read_message("Payload")
    local msg = mysql.mariadb_slow_query_grammar:match(log)
    if not msg then return -1 end

    msg.Type = "mariadb.slow-query"
    if truncate_sql and #msg.Payload > truncate_sql then
        msg.Payload = string.format("%s...", msg.Payload:sub(1, truncate_sql))
    end

    inject_message(msg)
    return 0
end

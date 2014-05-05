-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses and transforms the MySOL slow query logs.

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
    script_type = "lua"
    filename = "lua_decoder/mysql_slow_query.lua"

*Example Heka Message*

:Timestamp: 2013-03-28 14:41:03 -0700 PDT
:Type: mysql.slow-query
:Hostname: 10.14.214.12
:Pid: 0
:UUID: 316bc355-e7c2-4d02-a4ad-91ed03662e6f
:Logger: Sync-1_5-SlowQuery
:Payload: SELECT collection, MAX(modified) FROM wbo96 WHERE username='883496' GROUP BY username, collection;
:EnvVersion:
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

local mysql = require "mysql"

function process_message ()
    local log = read_message("Payload")
    local msg = mysql.slow_query_grammar:match(log)
    if not msg then return -1 end

    msg.Type = "mysql.slow-query"
    inject_message(msg)
    return 0
end

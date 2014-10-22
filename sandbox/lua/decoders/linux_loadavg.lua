-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a `/proc/loadavg` file into a Heka
message.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [LoadAvg]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/loadavg"
    decoder = "LoadAvgDecoder"

    [LoadAvgDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_loadavg.lua"

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: stats.loadavg
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"1MinAvg" value_type:DOUBLE value_double:"3.05"
    | name:"5MinAvg" value_type:DOUBLE value_double:"1.21"
    | name:"15MinAvg" value_type:DOUBLE value_double:"0.44"
    | name:"NumProcesses" value_type:DOUBLE value_double:"11"
    | name:"FilePath" value_string:"/proc/loadavg"

--]]

local l = require 'lpeg'
l.locale(l)

local num = (l.digit^1 * "." * l.digit^1) / tonumber
local loadavg = l.Cg(num, "1MinAvg") *
    l.space * l.Cg(num, "5MinAvg") *
    l.space * l.Cg(num, "15MinAvg")
local procs = l.Cg(l.digit^1 / tonumber, "NumProcesses") * "/" * l.digit^1
local latestPid = l.digit^1

local grammar = lpeg.Ct(loadavg * l.space * procs * l.space * latestPid)

local payload_keep = read_config("payload_keep")

local msg = {
    Type = "stats.loadavg",
    Payload = nil,
    Fields = nil
}

function process_message()
    local data = read_message("Payload")
    msg.Fields = grammar:match(data)

    if not msg.Fields then
        return -1
    end

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields.FilePath = read_message("Fields[FilePath]")
    inject_message(msg)
    return 0
end

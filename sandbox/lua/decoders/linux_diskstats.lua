-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a `/sys/block/$DISK/stat` file
(where `$DISK` is a disk identifier such as `sda`) into a Heka message struct.
This also tries to obtain the TickerInterval of the input it recieved the data
from, by extracting it from a message field named `TickerInterval`.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [DiskStats]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/sys/block/sda1/stat"
    decoder = "DiskStatsDecoder"

    [DiskStatsDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_diskstats.lua"

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: stats.diskstats
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"ReadsCompleted" value_type:DOUBLE value_double:"20123"
    | name:"ReadsMerged" value_type:DOUBLE value_double:"11267"
    | name:"SectorsRead" value_type:DOUBLE value_double:"1.094968e+06"
    | name:"TimeReading" value_type:DOUBLE value_double:"45148"
    | name:"WritesCompleted" value_type:DOUBLE value_double:"1278"
    | name:"WritesMerged" value_type:DOUBLE value_double:"1278"
    | name:"SectorsWritten" value_type:DOUBLE value_double:"206504"
    | name:"TimeWriting" value_type:DOUBLE value_double:"3348"
    | name:"TimeDoingIO" value_type:DOUBLE value_double:"4876"
    | name:"WeightedTimeDoingIO" value_type:DOUBLE value_double:"48356"
    | name:"NumIOInProgress" value_type:DOUBLE value_double:"3"
    | name:"TickerInterval" value_type:DOUBLE value_double:"2"
    | name:"FilePath" value_string:"/sys/block/sda/stat"

--]]

local l = require 'lpeg'
l.locale(l)

local space = l.space^0
local num = l.digit^1

function column(tag)
  return space * l.Cg(num / tonumber, tag)
end

local row = column("ReadsCompleted") * column("ReadsMerged") *
    column("SectorsRead") * column("TimeReading") * column("WritesCompleted") *
    column("WritesMerged") * column("SectorsWritten") * column("TimeWriting") *
    column("NumIOInProgress") * column("TimeDoingIO") *
    column("WeightedTimeDoingIO") * "\n"

local grammar = l.Ct(row)

local payload_keep = read_config("payload_keep")

local msg = {
    Type = "stats.diskstats",
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
    msg.Fields.TickerInterval = read_message("Fields[TickerInterval]")
    inject_message(msg)
    return 0
end

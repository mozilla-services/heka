-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a `/proc/meminfo` file into a Heka
message.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [MemStats]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/meminfo"
    decoder = "MemStatsDecoder"

    [MemStatsDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_memstats.lua"

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: stats.memstats
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"MemTotal" value_type:DOUBLE representation:"kB" value_double:"4047616"
    | name:"MemFree" value_type:DOUBLE representation:"kB" value_double:"3432216"
    | name:"Buffers" value_type:DOUBLE representation:"kB" value_double:"82028"
    | name:"Cached" value_type:DOUBLE representation:"kB" value_double:"368636"
    | name:"FilePath" value_string:"/proc/meminfo"


The total available fields can be found in `man procfs`. All fields are of type
double, and the representation is in kB (except for the HugePages fields). Here
is a full list of fields available:

MemTotal, MemFree, Buffers, Cached, SwapCached, Active, Inactive, Active(anon),
Inactive(anon), Active(file), Inactive(file), Unevictable, Mlocked, SwapTotal,
SwapFree, Dirty, Writeback, AnonPages, Mapped, Shmem, Slab, SReclaimable,
SUnreclaim, KernelStack, PageTables, NFS_Unstable, Bounce, WritebackTmp,
CommitLimit, Committed_AS, VmallocTotal, VmallocUsed, VmallocChunk,
HardwareCorrupted, AnonHugePages, HugePages_Total, HugePages_Free,
HugePages_Rsvd, HugePages_Surp, Hugepagesize, DirectMap4k, DirectMap2M,
DirectMap1G.

Note that your available fields may have a slight variance depending on the
system's kernel version.

--]]

local l = require 'lpeg'
l.locale(l)

local name = l.C((1 - l.P":")^1) * ":"
local value = l.Cg( l.digit^1 / tonumber, "value")
local unit = l.C"kB"
local repr = l.Cg("\n" * l.Cc"" + l.space * unit * "\n", "representation")
local line = l.Cg(name * l.space^1 * l.Ct(value * repr))
local grammar = l.Cf(l.Ct"" * line^1, rawset)

local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp = nil,
    Type = "stats.memstats",
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

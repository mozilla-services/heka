-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of file `/proc/stat`.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [ProcStats]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/stat"
    decoder = "ProcStatDecoder"

    [ProcStatDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_procstat.lua"

*Example Heka Message*

:Timestamp: 2014-12-10 22:38:24 +0000 UTC
:Type: stats.proc
:Hostname: yourhost.net
:Pid: 0
:Uuid: d2546942-7c36-4042-ad2e-f6bfdac11cdb
:Logger:
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"cpu" type:double value:[14384,125,3330,946000,333,0,356,0,0,0]
    | name:"cpu[1-#]" type:double value:[14384,125,3330,946000,333,0,356,0,0,0]
    | name:"ctxt" type:double value:2808304
    | name:"btime" type:double value:1423004780
    | name:"intr" type:double value:[14384,125,3330,0,0,0,0,0,0,0...0]
    | name:"processes" type:double value:3811
    | name:"procs_running" type:double value:1
    | name:"procs_blocked" type:double value:0
    | name:"softirq" type:double value:[288977,23,101952,19,13046,19217,7,...]

    Cpu fields:
        1    2    3      4    5        6     7         8       9       10
        user nice system idle [iowait] [irq] [softirq] [steal] [guest] [guestnice]
        Note: systems provide user, nice, system, idle. Other fields depend on
        kernel.

    intr
        This line shows counts of interrupts serviced since boot time, for
        each of the possible system interrupts. The first column is the total
        of all interrupts serviced including unnumbered architecture specific
        interrupts; each subsequent column is the total for that particular
        numbered interrupt. Unnumbered interrupts are not shown, only summed
        into the total.

--]]

local l = require 'lpeg'
l.locale(l)

local sp = l.P" "^1
local num = l.digit^1 / tonumber
local key = l.C((l.alnum + "_")^1)
local array = l.Ct((num * sp^0)^1)
local line =  l.Cg(key * sp * array) * l.P"\n"^0
local grammar = l.Cf(l.Ct("") * line^1, rawset)

local payload_keep = read_config("payload_keep")
local msg = {
    Type = "stats.procstat",
    Payload = nil,
    Fields = nil,
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
    inject_message(msg)
    return 0
end

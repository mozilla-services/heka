-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a 'A=`head -1 /proc/stat`; sleep 1; B=`head -1 /proc/stat`; echo ${A}zzz${B}zzz;' command into a Heka
message.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [stat_ProcessInput]
    type = "ProcessInput"
    decoder = "StatDecoder"
    ticker_interval = 3
    stdout = true
    stderr = false
        [stat_ProcessInput.command.0]
        bin = "/bin/sh"
        args = ["-c",'A=`head -1 /proc/stat`; sleep 1; B=`head -1 /proc/stat`; echo ${A}zzz${B}zzz;']


    [StatDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_stat.lua"


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


    user: Time spent executing user applications (user mode).
    nice: Time spent executing user applications with low priority (nice).
    system: Time spent executing system calls (system mode).
    idle: Idle time.
    iowait: Time waiting for I/O operations to complete.
    irq: Time spent servicing interrupts.
    softirq: Time spent servicing soft-interrupts.
    steal: Used in virtualization setups.
    guest: Used in virtualization setups.
    guest_nice: running a niced guest



--]]



local l = require 'lpeg'
l.locale(l)
local num = (l.digit^1) / tonumber
local cpu = l.Cg("cpu", 'cpu') * l.space^1
local stat = 
    l.Cg(num, "user") * l.space *
    l.Cg(num, "nice") * l.space *
    l.Cg(num, "system") * l.space *
    l.Cg(num, "idle") * l.space *
    l.Cg(num, "iowait") * l.space *
    l.Cg(num, "irq") * l.space *
    l.Cg(num, "softirq") * l.space *
    l.Cg(num, "steal") * l.space
--    guest: running a normal guest
--   guest_nice: running a niced guest
local cpudata = l.Ct(cpu * stat) * "0 0zzz"
local cpug = l.Cg(cpudata, 'cpu_global_1') * l.Cg(cpudata, 'cpu_global_2')
local grammar = l.Ct( cpug )

local payload_keep = read_config("payload_keep")

local msg = {
    Type = "stats.stat",
    Payload = nil,
    Fields = nil
}




function process_message()
    local data = read_message("Payload")
    local f = grammar:match(data);
    -- msg.Fields = grammar:match(data);

    --if not msg.Fields then
    if not f then
        return -1
    end

    if payload_keep then
        msg.Payload = data
    end
    
    -- TODO: handle overflows... the stat might overflow at max int causing the diff to be huge
    -- TODO: cleanup code
    msg.Fields = {}
    msg.Fields['d_user'] = f['cpu_global_2']['user'] - f['cpu_global_1']['user']
    msg.Fields['d_nice'] = f['cpu_global_2']['nice'] - f['cpu_global_1']['nice']
    msg.Fields['d_system'] = f['cpu_global_2']['system'] - f['cpu_global_1']['system']
    msg.Fields['d_idle'] = f['cpu_global_2']['idle'] - f['cpu_global_1']['idle']
    msg.Fields['d_iowait'] = f['cpu_global_2']['iowait'] - f['cpu_global_1']['iowait']
    msg.Fields['d_irq'] = f['cpu_global_2']['irq'] - f['cpu_global_1']['irq']
    msg.Fields['d_softirq'] = f['cpu_global_2']['softirq'] - f['cpu_global_1']['softirq']

    local total = msg.Fields['d_user'] + msg.Fields['d_nice'] + msg.Fields['d_system'] + msg.Fields['d_idle'] + msg.Fields['d_iowait'] + msg.Fields['d_irq'] + msg.Fields['d_softirq']
    
    msg.Fields['total'] = total
    if total > 0 then
        msg.Fields['total'] = total
        if msg.Fields['d_user'] > 0 then
            msg.Fields['p_user'] = (msg.Fields['d_user'] / total)
        end
        if msg.Fields['d_nice'] > 0 then
            msg.Fields['p_nice'] = msg.Fields['d_nice']  / total
        end
        if msg.Fields['d_system'] > 0 then
            msg.Fields['p_system'] = msg.Fields['d_system']  / total
        end
        if msg.Fields['d_idle'] > 0 then
            msg.Fields['p_idle'] = msg.Fields['d_idle']  / total
        end
    end
 
    
    msg.Fields.FilePath = read_message("Fields[FilePath]")
    inject_message(msg)
    return 0
end

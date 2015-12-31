-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Calculates deltas in /proc/stat data. Also emits CPU percentage utilization
information.

Config:

- whitelist (string, optional, default "")
    Only process fields that fit the `pattern <http://lua-
    users.org/wiki/PatternsTutorial>`_, defaults to match all.

- extras (boolean, optional, default false)
    Process extra fields like ctxt, softirq, cpu fields.

- percent_integer (boolean, optional, default true)
    Process percentage as whole number.


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

    [ProcStatFilter]
    type = "SandboxFilter"
    filename = "lua_filters/procstat.lua"
    preserve_data = true
    message_matcher = "Type == 'stats.procstat'"
    [ProcStatFilter.config]
        whitelist = "cpu$"
        extras = false
        percent_integer = true


Cpu fields:
    1    2    3      4    5        6     7         8       9       10
    user nice system idle [iowait] [irq] [softirq] [steal] [guest] [guestnice]
    Note: systems provide user, nice, system, idle. Other fields depend on kernel.

    user: Time spent executing user applications (user mode).
    nice: Time spent executing user applications with low priority (nice).
    system: Time spent executing system calls (system mode).
    idle: Idle time.
    iowait: Time waiting for I/O operations to complete.
    irq: Time spent servicing interrupts.
    softirq: Time spent servicing soft-interrupts.
    steal: ticks spent executing other virtual hosts [virtualization setups]
    guest: Used in virtualization setups.
    guestnice: running a niced guest

intr
    This line shows counts of interrupts serviced since boot time, for each of
    the possible system interrupts. The first column is the total of all
    interrupts serviced including unnumbered architecture specific interrupts;
    each subsequent column is the total for that particular numbered
    interrupt.  Unnumbered interrupts are not shown, only summed into the
    total.

ctxt 115315
    The number of context switches that the system underwent.

btime 769041601
    Boot time, in seconds since the Epoch, 1970-01-01 00:00:00 +0000 (UTC).

processes 86031
    Number of forks since boot.

procs_running 6
    Number of process in runnable state. (Linux 2.5.45 onward.)

procs_blocked 2
    Number of process blocked waiting for I/O to complete. (Linux 2.5.45
    onward.)

softirq 288977 23 101952 19 13046 19217 7 19125 92077 389 43122
    Time spent servicing soft-interrupts.
--]]

require 'string'
require 'table'
require 'math'

local proc_stat_previous = nil
local proc_stat_mappings = {
    cpu = {
        'user', 'nice', 'system', 'idle', 'iowait', 'irq',
        'softirq', 'steal', 'guest', 'guestnice'
    }
}
local whitelist = read_config('whitelist') or false
local extras = read_config('extras') or false
local percent_integer = read_config('percent_integer') or true


function read_fields()
    -- read fields_into a table
    local data = {}
    while true do
        local typ, name, value, rep, count = read_next_field()
        if not typ then break end
        if count == 1 then
            data[name] = value
        else
            local field_name = string.format("Fields[%s]", name)
            data[name] = {}
            data[name][1] = value
            for i=1, count - 1 do
                data[name][i+1] = read_message(field_name, 0, i)
            end
        end
    end
    return data
end


function calc_delta(new, old)
    -- Recursion - check diff in table or value
    if type(new) ~= type(old) then return nil end
    if type(new) ~= 'table' then return new - old end
    if table.getn(new) == table.getn(old) then
        local delta = {}
        for k, v in pairs(old) do
            delta[k] = calc_delta(new[k], v)
        end
        return delta
    end
    return nil
end


function format_percent(value)
    if percent_integer then
        return math.ceil((value * 100) - 0.5)
    end
    return value
end


function process_delta(delta)
    -- calculate fields of interest
    local fields = {}
    for k, v in pairs(delta) do
        if not whitelist or k:match(whitelist) then
            if k:match('^cpu') ~= nil then
                local ticks = 0
                for ii, vv in pairs(v) do
                    if extras then
                        local field_name = proc_stat_mappings['cpu'][ii]
                        fields[k..'_delta_'..field_name] = vv
                    end
                    ticks = ticks + vv
                end
                if extras then
                    fields[k..'_delta_ticks'] = ticks
                end
                if ticks > 0 then
                    if extras then
                        -- only default kernel fields: user, nice, system, idle
                        fields[k..'_percent_user'] = format_percent(v[1] / ticks)
                        fields[k..'_percent_nice'] = format_percent(v[2] / ticks)
                        fields[k..'_percent_system'] = format_percent(v[3] / ticks)
                        fields[k..'_percent_idle'] = format_percent(v[4] / ticks)
                    end
                    fields[k..'_percent'] = format_percent(1 - (v[4] / ticks))
                end
            elseif extras and k == 'softirq' then
                for ii, vv in pairs(v) do
                    fields[k..'_delta_'..ii] = vv
                end
            elseif extras and type(v) ~= 'table' then
                fields[k..'_delta'] = v
            end
        end
    end
    return fields
end


function process_message()
    local stats = {}
    local msg = {
        Type = 'procstat',
        Payload = read_message('Payload'), 
        Fields = {},
    }
    local data = read_fields()

    if not proc_stat_previous then
        proc_stat_previous = data
        return 0
    end

    local delta = calc_delta(data, proc_stat_previous)
    msg.Fields = process_delta(delta)
    proc_stat_previous = data
    inject_message(msg)
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Calculates deltas in /proc/stat data.
Also emits CPU percentage utilization information.

Config:

- sec_per_row (uint, optional, default 60)
    Sets the size of each bucket (resolution in seconds) in the sliding window.

- whitelist_pattern (string)
    Only proccess fields the fit the pattern, defaults to match the global cpu: "cpu$"
    see: http://lua-users.org/wiki/PatternsTutorial

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
        whitelist_pattern = "cpu*"


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
    This line shows counts of interrupts serviced since
    boot time, for each of the possible system interrupts.
    The first column is the total of all interrupts
    serviced including unnumbered architecture specific
    interrupts; each subsequent column is the total for
    that particular numbered interrupt.  Unnumbered
    interrupts are not shown, only summed into the total.

ctxt 115315
    The number of context switches that the system
    underwent.

btime 769041601
    boot time, in seconds since the Epoch, 1970-01-01
    00:00:00 +0000 (UTC).

processes 86031
    Number of forks since boot.

procs_running 6
    Number of processes in runnable state.  (Linux 2.5.45 onward.)

procs_blocked 2
    Number of processes blocked waiting for I/O to complete.  (Linux 2.5.45 onward.)

softirq 288977 23 101952 19 13046 19217 7 19125 92077 389 43122
    Time spent servicing soft-interrupts.
--]]

require 'string'

local proc_stat_previous = nil
local proc_stat_mappings = {
    cpu = {
        'user', 'nice', 'system', 'idle', 'iowait', 'irq',
        'softirq', 'steal', 'guest', 'guestnice'
    }
}

local preserve_data = read_config('preserve_data')
local blacklist_regex = read_config('blacklist_regex') or 'cpu$'

function process_delta(new, old)
    -- Recursion - check diff in table or value
    if type(new) ~= type(old) then return nil end
    if type(old) ~= 'table' then
        return new - old
    end
    local delta = {}
    for k, v in pairs(old) do
        delta[k] = process_delta(new[k], v)
    end
    return delta
end


function process_fields(delta)
    -- proccess deltas and calculate cpu average usage on fields of interest
    local fields = {}
    for k, v in pairs(delta) do
        --if type(v) ~= 'table' then
        --    msg.Fields[k..'_delta'] = v
        --end
        if k:match('cpu') ~= nil then
            local totalticks = 0
            for ii, vv in pairs(v) do
                local field_name = proc_stat_mappings['cpu'][ii]
                fields[k..'_delta_'..field_name] = vv
                totalticks = totalticks + vv
            end
            if totalticks > 0 then
                -- only pre-calc user, nice, system, idle because all kernels have field 
                fields[k..'_calc_totalticks'] = totalticks
                fields[k..'_calc_user'] = fields[k..'_delta_user'] / totalticks
                fields[k..'_calc_nice'] = fields[k..'_delta_nice'] / totalticks
                fields[k..'_calc_system'] = fields[k..'_delta_system'] / totalticks
                fields[k..'_calc_idle'] = fields[k..'_delta_idle'] / totalticks
                fields[k..'_calc_usage'] = 1 - fields[k..'_calc_idle']
            end
        end
        if k == 'ctext' then
            fields[k..'_delta'] = v
        end
        if k == 'softirq' then
            for ii, vv in pairs(v) do
                fields[k..'_delta_'..ii] = vv
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
    -- read values found in the fields_index into a usable table
    local wl = whitelist_pattern
    local data = {}
    local index_count = read_message('Fields[index_count]')
    if index_count == nil then return 0 end
    for i=1, index_count do
        f = read_message('Fields[index]', 0, i-1)
        if f == nil then break end
        -- only process fields in whitelist
        if wl == nil or (wl ~= nil and f:match(wl)) then
            check_array = read_message('Fields['..f..']', 0, 1)
            if check_array == nil then
                data[f] = read_message('Fields['..f..']')
            else
                data[f] = {}
                for ii=1, 500 do
                    ff = read_message('Fields['..f..']', 0, ii-1)
                    if ff == nil then break end
                    data[f][ii] = ff
                end
            end
        end
    end
    if proc_stat_previous == nil then
        proc_stat_previous = data
        return 0
    end
    local delta = process_delta(data, proc_stat_previous)
    msg.Fields = process_fields(delta)
    proc_stat_previous = data
    inject_message(msg)
    return 0
end

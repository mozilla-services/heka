-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a `/proc/net/net/dev` file into a
Heka message.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [Netdev]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/net/dev"
    decoder = "NetdevDecoder"

    [NetdevDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_netdev.lua"

*Example Heka Message*

:Timestamp: 2015-09-03 13:44:25 +0000 UTC
:Type: stats.netdev
:Hostname: ultrathieu
:Pid: 0
:Uuid: cf705300-b3d7-4e5a-a56e-37846f8c246a
:Logger: Netdev
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"lo_transmit_carrier" type:integer value:0
    | name:"eth0_receive_fifo" type:integer value:0
    | name:"lo_transmit_bytes" type:integer value:50278
    | name:"lo_receive_multicast" type:integer value:0
    | name:"eth0_receive_packets" type:integer value:0
    | name:"lo_transmit_compressed" type:integer value:0
    | name:"eth0_transmit_packets" type:integer value:0
    | name:"lo_transmit_colls" type:integer value:0
    | name:"eth0_transmit_compressed" type:integer value:0
    | name:"eth0_receive_drop" type:integer value:0
    | name:"eth0_receive_frame" type:integer value:0
    | name:"eth0_transmit_errs" type:integer value:0
    | name:"eth0_transmit_fifo" type:integer value:0
    | name:"lo_receive_drop" type:integer value:0
    | name:"eth0_receive_bytes" type:integer value:0
    | name:"lo_transmit_drop" type:integer value:0
    | name:"lo_receive_frame" type:integer value:0
    | name:"FilePath" type:string value:"/proc/net/dev"
    | name:"lo_transmit_fifo" type:integer value:0
    | name:"lo_transmit_errs" type:integer value:0
    | name:"eth0_transmit_drop" type:integer value:0
    | name:"lo_transmit_packets" type:integer value:601
    | name:"lo_receive_compressed" type:integer value:0
    | name:"lo_receive_fifo" type:integer value:0
    | name:"lo_receive_errs" type:integer value:0
    | name:"eth0_transmit_carrier" type:integer value:0
    | name:"lo_receive_packets" type:integer value:601
    | name:"lo_receive_bytes" type:integer value:50278
    | name:"eth0_transmit_colls" type:integer value:0
    | name:"eth0_receive_compressed" type:integer value:0
    | name:"eth0_receive_errs" type:integer value:0
    | name:"eth0_receive_multicast" type:integer value:0
    | name:"eth0_transmit_bytes" type:integer value:0

--]]

local table = require "table"
local l = require "lpeg"
l.locale(l)

local payload_keep = read_config("payload_keep")

local space = l.P' '^0
local keyword = (l.lower + l.digit)^1
local counter = l.Ct(l.Cg(l.digit^1 / tonumber, "value") * l.Cg(l.Cc'2', "value_type"))
local header = l.Ct(
               l.P'Inter-'
             * l.P'|'
             * space
             * l.P'Receive'
             * space
             * l.P'|'
             * space
             * l.P'Transmit'
             * l.P'\n'
             -- second line
             * space
             * l.P'face'
             * space
             * l.P'|'
             * space
             * l.Ct((l.Cg(keyword) * space)^1)
             * l.P'|'
             * space
             * l.Ct((l.Cg(keyword) * space)^1)
             * l.P'\n'
             )
local line = l.Ct(
             space
           * l.Cg(keyword)
           * l.P':'
           * space
           * l.Ct((l.Cg(counter) * space)^1)
           * l.P'\n'
           )

local grammar = l.Ct(
                header
              * l.Ct(line^1)
              )

local field_names = nil

local msg = {
    Timestamp = nil,
    Type = "stats.netdev",
    Payload = nil,
    Fields = nil
}

function process_message()
    local payload = read_message("Payload")

    local data = grammar:match(payload)

    if not data then
        return -1, "Failed to match grammar"
    end

    msg.Fields = {}

    if field_names == nil then
        -- Extract field names only once
        local header = data[1]
        local receive_fields = header[1]
        local transmit_fields = header[2]
        field_names = {}
        for i, v in ipairs(receive_fields) do
            table.insert(field_names, 'receive_' .. v)
        end
        for i, v in ipairs(transmit_fields) do
            table.insert(field_names, 'transmit_' .. v)
        end
    end
    local counters = data[2]
    for i, iface_counters in ipairs(counters) do
        local iface = iface_counters[1]
        local counters = iface_counters[2]
        for i2, v2 in ipairs(counters) do
            msg.Fields[iface .. '_' .. field_names[i2]] = v2
        end
    end

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields.FilePath = read_message("Fields[FilePath]")
    inject_message(msg)
    return 0
end

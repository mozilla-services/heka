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

:Timestamp: 2015-10-16 13:31:07 +0000 UTC
:Type: stats.netdev
:Hostname: example.com
:Pid: 0
:Uuid: 505561dc-81f6-4856-abe5-077c24457010
:Logger: NetdevInput
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"receive_multicast" type:double value:0
    | name:"transmit_errs" type:double value:0
    | name:"receive_drop" type:double value:0
    | name:"netdevice" type:string value:"eth0"
    | name:"transmit_drop" type:double value:0
    | name:"transmit_carrier" type:double value:0
    | name:"receive_packets" type:double value:1.18443194e+08
    | name:"receive_compressed" type:double value:0
    | name:"transmit_colls" type:double value:0
    | name:"transmit_compressed" type:double value:0
    | name:"receive_frame" type:double value:0
    | name:"transmit_packets" type:double value:1.07330545e+08
    | name:"receive_fifo" type:double value:0
    | name:"receive_bytes" type:double value:1.3915983085e+11
    | name:"transmit_bytes" type:double value:1.78516842512e+11
    | name:"receive_errs" type:double value:0
    | name:"transmit_fifo" type:double value:0

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

    if payload_keep then
        msg.Payload = data
    end

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
        local fields = {}
        fields.FilePath = read_message("Fields[FilePath]")

        local iface = iface_counters[1]
        local counters = iface_counters[2]
        for i2, v2 in ipairs(counters) do
            fields[field_names[i2]] = v2
        end

        fields.netdevice = iface

        msg.Fields = fields
        inject_message(msg)
    end

    return 0
end

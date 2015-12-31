-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses a payload containing the contents of a `/proc/net/netstat` or
`/proc/net/snmp` file into a Heka message.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [NetNetstat]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/net/netstat"
    decoder = "NetstatDecoder"

    [NetSnmp]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/net/snmp"
    decoder = "NetstatDecoder"

    [NetstatDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_netstat.lua"

*Example Heka Message*

:Timestamp: 2015-08-28 15:52:00 +0000 UTC
:Type: stats.netstat
:Hostname: test.example.com
:Pid: 0
:Uuid: 90c202d1-1375-4ec2-ac8c-eb53b2850d19
:Logger: NetSnmp
:Payload: 
:EnvVersion: 
:Severity: 7
:Fields:
    | name:"Ip_FragCreates" type:integer value:0
    | name:"Ip_FragOKs" type:integer value:0
    | name:"Icmp_InTimestamps" type:integer value:0
    | name:"Ip_InUnknownProtos" type:integer value:0
    | name:"Ip_ReasmFails" type:integer value:0
    | name:"Icmp_OutErrors" type:integer value:0
    | name:"Icmp_InDestUnreachs" type:integer value:19812
    | name:"Ip_InReceives" type:integer value:718979
    | name:"Ip_ReasmTimeout" type:integer value:0
    | name:"Ip_InHdrErrors" type:integer value:0
    | name:"Ip_ReasmOKs" type:integer value:0
    | name:"Icmp_OutSrcQuenchs" type:integer value:0
    | name:"Icmp_InAddrMaskReps" type:integer value:0
    | name:"Ip_OutNoRoutes" type:integer value:1788
    | name:"IcmpMsg_OutType0" type:integer value:81
    | name:"Ip_FragFails" type:integer value:0
    | name:"Icmp_OutTimeExcds" type:integer value:0
    | name:"Ip_ReasmReqds" type:integer value:0
    | name:"IcmpMsg_InType3" type:integer value:19812
    | name:"Ip_InDiscards" type:integer value:0
    | name:"Icmp_InTimestampReps" type:integer value:0
    | name:"Icmp_InEchoReps" type:integer value:0
    | name:"Icmp_OutAddrMasks" type:integer value:0
    | name:"Icmp_InMsgs" type:integer value:19893
    | name:"Icmp_OutMsgs" type:integer value:19892
    | name:"Icmp_OutTimestampReps" type:integer value:0
    | name:"Icmp_InSrcQuenchs" type:integer value:0
    | name:"IcmpMsg_OutType3" type:integer value:19811
    | name:"Icmp_OutEchoReps" type:integer value:81
    | name:"Icmp_OutParmProbs" type:integer value:0
    | name:"Icmp_OutRedirects" type:integer value:0
    | name:"Icmp_OutEchos" type:integer value:0
    | name:"Ip_DefaultTTL" type:integer value:64
    | name:"Icmp_InCsumErrors" type:integer value:0
    | name:"IcmpMsg_InType8" type:integer value:81
    | name:"Icmp_InRedirects" type:integer value:0
    | name:"Ip_OutDiscards" type:integer value:9272
    | name:"FilePath" type:string value:"/proc/net/snmp"
    | name:"Icmp_InErrors" type:integer value:0
    | name:"Ip_Forwarding" type:integer value:1
    | name:"Icmp_OutTimestamps" type:integer value:0
    | name:"Icmp_InEchos" type:integer value:81
    | name:"Icmp_InAddrMasks" type:integer value:0
    | name:"Icmp_InTimeExcds" type:integer value:0
    | name:"Ip_OutRequests" type:integer value:544286
    | name:"Ip_InDelivers" type:integer value:718236
    | name:"Ip_InAddrErrors" type:integer value:31
    | name:"Icmp_OutAddrMaskReps" type:integer value:0
    | name:"Ip_ForwDatagrams" type:integer value:0
    | name:"Icmp_InParmProbs" type:integer value:0
    | name:"Icmp_OutDestUnreachs" type:integer value:19811

--]]

local l = require "lpeg"
l.locale(l)

local payload_keep = read_config("payload_keep")

local keyword = (l.alpha + l.digit)^1
local counter = l.Ct(l.Cg(l.digit^1 / tonumber, "value") * l.Cg(l.Cc'2', "value_type"))
local fields_line = l.Ct(
                    l.C(keyword)
                  * l.P':'
                  * l.Ct((l.P" " * l.C(keyword))^1)
                  * l.P"\n"
                  )
local values_line = l.Ct(
                    l.C(keyword)
                  * l.P':'
                  * l.Ct((l.P" " * counter)^1)
                  * l.P"\n"
                  )
local grammar = l.Ct(l.Ct(fields_line * values_line)^1)

local msg = {
    Timestamp = nil,
    Type = "stats.netstat",
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

    for k, proto_data in ipairs(data) do
        local proto = proto_data[1][1]
        local fields = proto_data[1][2]
        local values = proto_data[2][2]
        for k2, v2 in pairs(fields) do
            msg.Fields[proto .. '_' .. v2] = values[k2]
        end
    end

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields.FilePath = read_message("Fields[FilePath]")
    inject_message(msg)
    return 0
end

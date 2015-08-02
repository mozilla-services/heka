-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the Microsoft sharepoint uls logs based on the uls log format.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

[hekad]
share_dir = 'C:\heka-agent\heka\\share\heka'
base_dir = 'C:\var\cache\hekad'

[SharePointULSLogs]
type = "LogstreamerInput"
log_directory = 'F:\Trace_log'
file_match = '(?P<first>\w+)-(?P<second>\S+)-(?P<Year>\d{4})(?P<Month>\d{2})(?P<Day>\d{2})-(?P<time>\d+).log'
priority = ["Year","Month","Day","time"]
decoder = "SharePointDecoder"

[SharePointDecoder]
type = "SandboxDecoder"
script_type = "lua"
filename = 'lua_decoders\sharepoint_uls.lua'

[SharePointDecoder.config]
payload_keep = true
tz = "Local"

*Example Heka Message*

2015/08/02 01:05:58
:Timestamp: 2015-04-21 00:04:46 +0000 UTC
:Type: uls
:Hostname: sharepoint-host
:Pid: 0
:Uuid: fb4f1f82-7c8b-4cdc-aa10-6e7f76cb2a0e
:Logger: SharePointULSLogs
:Payload: 04/20/2015 20:04:46.02        OWSTIMER.EXE (0x5A0C)                           0x7D8C  SharePoint Foundation           Monitoring
                b4ly    Medium          Leaving Monitored Scope (Timer Job Search Health Monitoring - Trace Events). Execution Time=328.687724277806
76b7fe9c-03c0-40dc-2ea6-6162b7e29775

:EnvVersion:
:Severity: 7
:Fields:
    | name:"Correlation" type:string value:"76b7fe9c-03c0-40dc-2ea6-6162b7e29775
"
    | name:"TID" type:string value:"0x7D8C"
    | name:"EventID" type:string value:"b4ly"
    | name:"Message" type:string value:"Leaving Monitored Scope (Timer Job Search Health Monitoring - Trace Events). Execution Time=328.687724277806"
    | name:"Category" type:string value:"Monitoring                    "
    | name:"Level" type:string value:"Medium  "
    | name:"Process" type:string value:"OWSTIMER.EXE (0x5A0C)                   "
    | name:"Area" type:string value:"SharePoint Foundation         "

--]]

local dt = require "date_time"
local l = require 'lpeg'
l.locale(l)

local sp = l.space
local num = l.digit^1 / tonumber

function extract_quote(openp,endp)
    openp = l.P(openp)
    endp = endp and l.P(endp) or openp
    local upto_endp = (1 - endp)^0 
    return openp * l.C(upto_endp) * endp
end

local datetime = dt.build_strftime_grammar("%m/%d/%Y %H:%M:%S") / dt.time_to_ns * "." * l.R("09","*/")^0
local process = l.Cg(extract_quote(sp^1, "\t"), "Process")
local t_id = l.Cg(extract_quote("", "\t"), "TID")
local area = l.Cg(extract_quote("", "\t"), "Area")
local category = l.Cg(extract_quote("", "\t"), "Category")
local event_id = l.Cg(extract_quote("", "\t"), "EventID")
local level = l.Cg(extract_quote("", "\t"), "Level")
local message = l.Cg(extract_quote("", "\t"), "Message")
local correlation = l.Cg(l.P(1)^0, "Correlation")

local request = l.Cg(datetime,"DateTime") * process * t_id * area * category * event_id * level * message * correlation

grammar = l.Ct(request)

local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp = nil,
    Payload = nil,
    Hostname = nil,
    Fields = nil,
    Type = "uls"
}

function process_message()

    local data = read_message("Payload")
    local host = read_message("Hostname")
    local fields = grammar:match(data)
    
    if not fields then
	return -1
    end

    msg.Timestamp = fields.DateTime
    msg.Hostname = string.lower(host)
    fields.DateTime = nil
    msg.Fields = fields

    if payload_keep then
        msg.Payload = data
    end
    
    inject_message(msg)
    return 0
end

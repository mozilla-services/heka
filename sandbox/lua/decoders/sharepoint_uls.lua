-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Parses the Microsft sharepoint uls logs based on the uls log format.


*Example Heka Configuration*

.. code-block:: ini

[hekad]
share_dir="C:\\heka-agent\\heka\\share\\heka"
base_dir = "C:\\var\\cache\\hekad"

[SharePointULSLogs]
type = "LogstreamerInput"
log_directory = "F:\\Trace_log"
file_match = '(?P<first>\w+)-(?P<second>\S+)-(?P<Year>\d{4})(?P<Month>\d{2})(?P<Day>\d{2})-(?P<time>\d+).log'
priority = ["Year","Month","Day","time"]
decoder = "SharePointDecoder"

[SharePointDecoder]
type = "SandboxDecoder"
script_type = "lua"
filename = "lua_decoders\\sharepoint.lua"

[SharePointDecoder.config]
payload_keep = true
tz = "Local"

*Example Heka Message*

--]]

local dt = require "date_time"
local l = require 'lpeg'
l.locale(l)

local sp = l.space
num = l.digit^1 / tonumber

function extract_quote(openp,endp)
    openp = l.P(openp)
    endp = endp and l.P(endp) or openp
    local upto_endp = (1 - endp)^0 
    return openp * l.C(upto_endp) * endp
end

local sp = l.space

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

function process_message()

local msg = {
    Timestamp = nil,
    Payload = nil,
    Hostname = nil,
    Fields = nil,
    Type = nil
}

    local data = read_message("Payload")
    local host = read_message("Hostname")
    local fields = grammar:match(data)
    
    if not fields then
	return -1
    end
    msg.Type = "uls"
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

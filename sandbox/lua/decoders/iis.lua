-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[[hekad]

Parses the iis logs based on the Microsoft iis log formats. This decoder is tested for iis verions 7 and 8. 


Config:

iis_version_7 = true
     Default configuration asssumes iis log format for version 8. 
     For version 7 and similar formats, set the decoder config variable  iis_version_7 to true

*Example Heka Configuration*

.. code-block:: ini


share_dir="C:\\heka-agent\\heka\\share\\heka"
base_dir = "C:\\var\\cache\\hekad"

[IISLogs]
type = "LogstreamerInput"
log_directory = "F:\\Web_Logs"
file_match = '(?P<dir>\w+)(?P<s>\S+)u_ex(?P<Index>\d+)\.log'
differentiator = ["dir"]
priority = ["Index"]
decoder = "IISDecoder"

[IISDecoder]
type = "SandboxDecoder"
script_type = "lua"
filename = "lua_decoders\\iis.lua"

[IISDecoder.config]
payload_keep = true
iis_version_7 = true
tz = "UTC"

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
    local upto_endp = (1 - endp)^1 
    return openp * l.C(upto_endp) * endp
end

local sp = l.space

local timestamp = l.Cg(dt.build_strftime_grammar("%Y-%m-%d %H:%M:%S") / dt.time_to_ns, "timestamp")
local host_ip = l.Cg(extract_quote(" ", " "), "host_ip")
local cs_method = l.Cg(extract_quote("", " "), "cs_method")
local cs_uri_stem = l.Cg(extract_quote("", " "), "cs_uri_stem")
local cs_uri_query = l.Cg(extract_quote("", " "), "cs_uri_query")
local port = l.Cg(extract_quote("", " "), "port")
local cs_username = l.Cg(extract_quote("", " "), "cs_username") 
local client_ip = l.Cg(extract_quote("", " "), "client_ip") 
local cs_user_agent = l.Cg(extract_quote("", " "), "cs_user_agent") 
local cs_referer = l.Cg(extract_quote("", " "), "cs_referer") 
local status = l.Cg(num, "status") 
local substatus = l.Cg(extract_quote(" ", " "), "substatus")
local win32_status = l.Cg(extract_quote("", " "), "win32_status")
local time_taken = l.Cg(num, "time_taken")
local version_8 = timestamp * host_ip * cs_method * cs_uri_stem * cs_uri_query * port * cs_username * client_ip * cs_user_agent * cs_referer * status * substatus * win32_status * time_taken
local version_7 = timestamp * host_ip * cs_method * cs_uri_stem * cs_uri_query * port * cs_username * client_ip * cs_user_agent * status * substatus * win32_status * time_taken 

grammar = l.Ct(version_8)


local iis_version = read_config("iis_version_7")

if iis_version then
  grammar = l.Ct(version_7)
end

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
    msg.Type = "iis"
    msg.Timestamp = fields.timestamp
    msg.Hostname = string.lower(host)
    fields.timestamp = nil
    msg.Fields = fields
    if msg.Fields.cs_username == "-" then
	msg.Fields.cs_username = ""
    end
    if msg.Fields.cs_uri_query == "-" then
	msg.Fields.cs_uri_query = ""
    end


    if payload_keep then
        msg.Payload = data
    end
    
    inject_message(msg)
    return 0
end

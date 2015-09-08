-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the iis logs based on the Microsoft iis log formats. This decoder
is tested for iis verions 7 and 8.

Config:

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

- iis_version_7 (bool, optional, default flase)
     Default configuration asssumes iis log format for version 8. For version
     7 and similar formats, set this to true.

*Example Heka Configuration*

.. code-block:: ini

[hekad]
share_dir = 'C:\heka-agent\heka\share\heka'
base_dir = 'C:\var\cache\hekad'

[IISLogs]
type = "LogstreamerInput"
log_directory = 'F:\Web_Logs'
file_match = '(?P<dir>\w+)(?P<s>\S+)u_ex(?P<Index>\d+)\.log'
differentiator = ["dir"]
priority = ["Index"]
decoder = "IISDecoder"

[IISDecoder]
type = "SandboxDecoder"
script_type = "lua"
filename = 'lua_decoders\iis.lua'

[IISDecoder.config]
payload_keep = true
iis_version_7 = true
tz = "UTC"

*Example Heka Message*

2015/08/02 00:34:43
:Timestamp: 2014-09-22 06:32:29 +0000 UTC
:Type: iis
:Hostname: iis-host
:Pid: 0
:Uuid: 2dd1d363-02e2-4d61-ade8-e4ed6657fcd6
:Logger: W3SVC1368505715
:Payload: 2014-09-22 06:32:29 101.181.48.45 GET / - 6005 - 10.181.72.190 Mozilla/4.0+(compatible;+MSIE+8.0;+Windows+NT+5.1;+Trident/4.0) 401 0 64 46

:EnvVersion:
:Severity: 7
:Fields:
    | name:"substatus" type:string value:"0"
    | name:"remote_addr" type:string value:"101.181.72.190"
    | name:"cs_method" type:string value:"GET"
    | name:"remote_user" type:string value:""
    | name:"user_agent" type:string value:"Mozilla/4.0+(compatible;+MSIE+8.0;+Windows+NT+5.1;+Trident/4.0)"
    | name:"time_taken" type:double value:46
    | name:"port" type:string value:"6005"
    | name:"win32_status" type:string value:"64"
    | name:"status" type:double value:401
    | name:"cs_uri_stem" type:string value:"/"
    | name:"host_ip" type:string value:"10.181.48.45"
    | name:"cs_uri_query" type:string value:""
    | name:"request" type:string value:"GET /"

--]]

local clf = require "common_log_format"
local dt = require "date_time"
local l = require "lpeg"
local string = require "string"
l.locale(l)

local iis_version = read_config("iis_version_7")
local payload_keep = read_config("payload_keep")

local sp = l.space
local num = l.digit^1 / tonumber

local function extract_quote(openp,endp)
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
local cs_username = l.Cg(extract_quote("", " "), "remote_user")
local client_ip = l.Cg(extract_quote("", " "), "remote_addr")
local cs_user_agent = l.Cg(extract_quote("", " "), "http_user_agent")
local cs_referer = l.Cg(extract_quote("", " "), "http_referer")
local status = l.Cg(num, "status")
local substatus = l.Cg(extract_quote(" ", " "), "substatus")
local win32_status = l.Cg(extract_quote("", " "), "win32_status")
local time_taken = l.Cg(num, "time_taken")
local version_8 = timestamp * host_ip * cs_method * cs_uri_stem * cs_uri_query * port * cs_username * client_ip * cs_user_agent * cs_referer * status * substatus * win32_status * time_taken
local version_7 = timestamp * host_ip * cs_method * cs_uri_stem * cs_uri_query * port * cs_username * client_ip * cs_user_agent * status * substatus * win32_status * time_taken

local grammar = l.Ct(version_8)

if iis_version then
  grammar = l.Ct(version_7)
end

local msg = {
    Timestamp = nil,
    Payload = nil,
    Hostname = nil,
    Fields = nil,
    Type = "iis"
}

function process_message()

    local data = read_message("Payload")
    local host = read_message("Hostname")
    local fields = grammar:match(data)

    if not fields then
      return -1
    end

    msg.Timestamp = fields.timestamp
    msg.Hostname = string.lower(host)
    fields.timestamp = nil

    if fields.cs_username == "-" then
	fields.cs_username = ""
    end

    if fields.cs_uri_query == "-" then
	fields.cs_uri_query = ""
    end

    if not fields.cs_uri_query or fields.cs_uri_query == "" then
        fields.request = string.format("%s %s", fields.cs_method, fields.cs_uri_stem)
    else
        fields.request = string.format("%s %s?%s", fields.cs_method, fields.cs_uri_stem, fields.cs_uri_query)
    end

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields = fields
    inject_message(msg)
    return 0
end

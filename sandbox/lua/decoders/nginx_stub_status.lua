-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses a payload containing the output of nginx's stub status module:
http://nginx.org/en/docs/http/ngx_http_stub_status_module.html

Config:

- type (string, optional, default 'nginx_stub_status')
    Set the message type.

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [NginxStubStatusInput]
    type = "HttpInput"
    url = "http://localhost:8090/nginx_status"
    ticker_interval = 1
    success_severity = 6
    error_severity = 1
    decoder = "NginxStubStatusDecoder"

    [NginxStubStatusDecoder]
    filename = "lua_decoders/nginx_stub_status.lua"
    type = "SandboxDecoder"

    [NginxStubStatusDecoder.config]
    payload_keep = false

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: nginx_stub_status
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"connections" value_type:INTEGER value_integer:"291"
    | name:"accepts" value_type:INTEGER value_integer:"16630948"
    | name:"handled" value_type:INTEGER value_integer:"16630948"
    | name:"requests" value_type:INTEGER value_integer:"31070465"
    | name:"reading" value_type:INTEGER value_integer:"6"
    | name:"writing" value_type:INTEGER value_integer:"179"
    | name:"waiting" value_type:INTEGER value_integer:"106"
--]]

local l = require 'lpeg'
l.locale(l)

local sp = l.S(' \n\t')^0
local num = l.digit^1
local headings = l.P'server accepts handled requests'

function column(tag)
  return sp * l.Cg(num / tonumber, tag)
end

local act_conn = l.P'Active connections:' * column('connections')

local reading = sp * l.P'Reading:' * column('reading')
local writing = sp * l.P'Writing:' * column('writing')
local waiting = sp * l.P'Waiting:' * column('waiting')

local fmt = act_conn * sp * headings * column('accepts') * column('handled') *
            column('requests') * reading * writing * waiting

local grammar = l.Ct(fmt)

local payload_keep = read_config("payload_keep")

local msg = {
    Type = read_config('type') or 'nginx_stub_status',
    Payload = nil,
    Fields = nil
}

function process_message()
    local data = read_message('Payload')
    msg.Fields = grammar:match(data)

    if not msg.Fields then
        return -1
    end

    if payload_keep then
        msg.Payload = data
    end

    inject_message(msg)
    return 0
end

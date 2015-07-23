-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the Nginx error logs based on the Nginx hard coded internal format.

Config:

- tz (string, optional, defaults to UTC)
    The conversion actually happens on the Go side since there isn't good TZ support here.

- type (string, optional, defaults to "nginx.error"):
    Sets the message 'Type' header to the specified value

*Example Heka Configuration*

.. code-block:: ini

    [TestWebserverError]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = 'error\.log'
    decoder = "NginxErrorDecoder"

    [NginxErrorDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/nginx_error.lua"

    [NginxErrorDecoder.config]
    tz = "America/Los_Angeles"

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: nginx.error
:Hostname: trink-x230
:Pid: 16842
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Logger: TestWebserverError
:Payload: using inherited sockets from "6;"
:EnvVersion:
:Severity: 5
:Fields:
    | name:"tid" value_type:DOUBLE value_double:0
    | name:"connection" value_type:DOUBLE value_double:8878
--]]

local clf = require "common_log_format"

local msg_type = read_config("type") or "nginx.error"

function process_message ()
    local log = read_message("Payload")
    local msg = clf.nginx_error_grammar:match(log)
    if not msg then return -1 end

    msg.Type = msg_type
    inject_message(msg)
    return 0
end

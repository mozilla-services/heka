-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the Nginx error logs based on the Nginx hard coded internal format.

Config:

- tz (string, optional, defaults to UTC)
    The conversion actually happens on the Go side since there isn't good TZ support here.

*Example Heka Configuration*

.. code-block:: ini

    [NginxErrorDecoder]
    type = "SandboxDecoder"
    script_type = "lua"
    filename = "lua_decoders/nginx_error.lua"

    [NginxErrorDecoder.config]
    tz = "America/Los_Angeles"

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: nginx.error
:Hostname: trink-x230
:Pid: 16842
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Logger: FxaWebserverError
:Payload: using inherited sockets from "6;"
:EnvVersion:
:Severity: 5
:Fields:
    | name:"tid" value_type:DOUBLE value_double:0
    | name:"connection" value_type:DOUBLE value_double:8878
--]]

local clf = require "common_log_format"

function process_message ()
    local log = read_message("Payload")
    local msg = clf.nginx_error_grammar:match(log)
    if not msg then return -1 end

    msg.Type = "nginx.error"
    inject_message(msg)
    return 0
end

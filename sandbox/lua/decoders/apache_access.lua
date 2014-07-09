-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the Apache access logs based on the Apache 'LogFormat' configuration
directive. The Apache format specifiers are mapped onto the Nginx variable
names where applicable e.g. %a -> remote_addr.  This allows generic web
filters and outputs to work with any HTTP server input.

Config:

- log_format (string)
    The 'LogFormat' configuration directive from the apache2.conf.
    %t variables are converted to the number of nanosecond since the Unix epoch
    and used to set the Timestamp on the message.
    http://httpd.apache.org/docs/2.4/mod/mod_log_config.html

- type (string, optional, default nil):
    Sets the message 'Type' header to the specified value

- user_agent_transform (bool, optional, default false)
    Transform the http_user_agent into user_agent_browser, user_agent_version,
    user_agent_os.

- user_agent_keep (bool, optional, default false)
    Always preserve the http_user_agent value if transform is enabled.

- user_agent_conditional (bool, optional, default false)
    Only preserve the http_user_agent value if transform is enabled and fails.

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example Heka Configuration*

.. code-block:: ini

    [TestWebserver]
    type = "LogstreamerInput"
    log_directory = "/var/log/apache"
    file_match = 'access\.log'
    decoder = "CombinedLogDecoder"

    [CombinedLogDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/apache_access.lua"

    [CombinedLogDecoder.config]
    type = "combined"
    user_agent_transform = true
    # combined log format
    log_format = '%h %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"'

    # common log format
    # log_format = '%h %l %u %t \"%r\" %>s %O'

    # vhost_combined log format
    # log_format = '%v:%p %h %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"'

    # referer log format
    # log_format = '%{Referer}i -> %U'

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: combined
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Logger: TestWebserver
:Payload:
:EnvVersion:
:Severity: 7
:Fields:
    | name:"remote_user" value_string:"-"
    | name:"http_x_forwarded_for" value_string:"-"
    | name:"http_referer" value_string:"-"
    | name:"body_bytes_sent" value_type:DOUBLE representation:"B" value_double:82
    | name:"remote_addr" value_string:"62.195.113.219" representation:"ipv4"
    | name:"status" value_type:DOUBLE value_double:200
    | name:"request" value_string:"GET /v1/recovery_email/status HTTP/1.1"
    | name:"user_agent_os" value_string:"FirefoxOS"
    | name:"user_agent_browser" value_string:"Firefox"
    | name:"user_agent_version" value_type:DOUBLE value_double:29
--]]

local clf = require "common_log_format"

local log_format    = read_config("log_format")
local msg_type      = read_config("type")
local uat           = read_config("user_agent_transform")
local uak           = read_config("user_agent_keep")
local uac           = read_config("user_agent_conditional")
local payload_keep  = read_config("payload_keep")

local msg = {
Timestamp   = nil,
Type        = msg_type,
Payload     = nil,
Fields      = nil
}

local grammar = clf.build_apache_grammar(log_format)

function process_message ()
    local log = read_message("Payload")
    local fields = grammar:match(log)
    if not fields then return -1 end

    msg.Timestamp = fields.time
    fields.time = nil

    if payload_keep then
        msg.Payload = log
    end

    if fields.http_user_agent and uat then
        fields.user_agent_browser,
        fields.user_agent_version,
        fields.user_agent_os = clf.normalize_user_agent(fields.http_user_agent)
        if not ((uac and not fields.user_agent_browser) or uak) then
            fields.http_user_agent = nil
        end
    end

    msg.Fields = fields
    inject_message(msg)
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

-- Parses the Nginx access logs based on the Nginx 'log_format' configuration
-- directive.
--
-- Example Heka Configuration:
--
--  [NginxAccessDecoder]
--  type = "SandboxDecoder"
--  script_type = "lua"
--  filename = "lua_decoders/nginx_access.lua"
--
--  [NginxAccessDecoder.config]
--  type = "fxa-auth-server"  # (string)
--      # Sets the message 'Type' header to the specified value
--  log_format = '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent" "$http_x_forwarded_for"' # (string)
--      # The 'log_format' configuration directive from the nginx.conf.
--      # $time_local or $time_iso8601 variable is converted to the number of
--      # nanosecond since the Unix epoch and used to set the Timestamp on the
--      # message.
--
-- Example Heka Message:
--
--  Timestamp: 2014-01-10 07:04:56 -0800 PST
--  Type: fxa-auth-server
--  Hostname: trink-x230
--  Pid: 0
--  UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
--  Logger: FxaNginxAccessInput
--  Payload:
--  EnvVersion:
--  Severity: 0
--  Fields: [
--  name:"remote_user" value_string:"-"
--  name:"http_x_forwarded_for" value_string:"-"
--  name:"http_referer" value_string:"-"
--  name:"body_bytes_sent" value_type:DOUBLE representation:"B" value_double:82
--  name:"remote_addr" value_string:"62.195.113.219"
--  name:"status" value_type:DOUBLE value_double:200
--  name:"http_user_agent" value_string:"Mozilla/5.0 (Mobile; rv:29.0) Gecko/29.0 Firefox/29.0"
--  name:"request" value_string:"GET /v1/recovery_email/status HTTP/1.1"
--  ]

local clf = require "common_log_format"

local log_format = read_config("log_format")
local msg_type = read_config("type")

local msg = {
Timestamp = nil,
Type = msg_type,
Fields = nil
}

local grammar = clf.build_nginx_grammar(log_format)

function process_message ()
    local log = read_message("Payload")
    local fields = grammar:match(log)
    if not fields then return -1 end

    msg.Timestamp = fields.time
    fields.time = nil

    msg.Fields = fields
    inject_message(msg)
    return 0
end

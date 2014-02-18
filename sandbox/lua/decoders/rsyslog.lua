-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the rsyslog output using the string based configuration template.

Config
~~~~~~
- template (string)
    The 'template' configuration string from rsyslog.conf.

- tz (string, optional, defaults to UTC)
    The conversion actually happens on the Go side since there isn't good TZ support here.

*Example Heka Configuration*

.. code-block:: ini

    [RsyslogDecoder]
    type = "SandboxDecoder"
    script_type = "lua"
    filename = "lua_decoders/rsyslog.lua"

    [RsyslogDecoder.config]
    template = '%TIMESTAMP% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n'
    tz = "America/Los_Angeles"

*Example Heka Message*

:Timestamp: 2014-02-10 12:58:58 -0800 PST
:Type: logfile
:Hostname: trink-x230
:Pid: 0
:UUID: e0eef205-0b64-41e8-a307-5772b05e16c1
:Logger: RsyslogInput
:Payload: "imklog 5.8.6, log source = /proc/kmsg started."
:EnvVersion:
:Severity: 7
:Fields:
    | name:"syslogtag" value_string:"kernel:"]
--]]

local syslog = require "syslog"

local template = read_config("template")

local msg = {
Timestamp   = nil,
Hostname    = nil,
Payload     = nil,
Severity    = nil,
Fields      = nil
}

local grammar = syslog.build_rsyslog_grammar(template)

function process_message ()
    local log = read_message("Payload")
    local fields = grammar:match(log)
    if not fields then return -1 end

    if fields.timestamp then
        msg.Timestamp = fields.timestamp
        fields.timestamp = nil
    end

    if fields.pri then
        msg.Severity = fields.pri.severity
        fields.syslogfacility = fields.pri.facility
        fields.pri = nil
    else
        msg.Severity = fields.syslogseverity or fields["syslogseverity-text"]
        or fields.syslogpriority or fields["syslogpriority-text"]

        fields.syslogseverity = nil
        fields["syslogseverity-text"] = nil
        fields.syslogpriority = nil
        fields["syslogpriority-text"] = nil
    end

    msg.Hostname = fields.hostname or fields.source
    fields.hostname = nil
    fields.source = nil

    msg.Payload = fields.msg
    fields.msg = nil

    msg.Fields = fields
    inject_message(msg)
    return 0
end

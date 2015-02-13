-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the rsyslog output using the string based configuration template.

Config:

- hostname_keep (boolean, defaults to false)
    Always preserve the original 'Hostname' field set by Logstreamer's 'hostname' configuration setting.
- template (string)
    The 'template' configuration string from rsyslog.conf.
    http://rsyslog-5-8-6-doc.neocities.org/rsyslog_conf_templates.html

- tz (string, optional, defaults to UTC)
    If your rsyslog timestamp field in the template does not carry zone offset information, you may set an offset
    to be applied to your events here. Typically this would be used with the "Traditional" rsyslog formats.

    Parsing is done by `Go <http://golang.org/pkg/time/#LoadLocation>`_, supports values of "UTC", "Local",
    or a location name corresponding to a file in the IANA Time Zone database, e.g. "America/New_York".

*Example Heka Configuration*

.. code-block:: ini

    [RsyslogDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/rsyslog.lua"

    [RsyslogDecoder.config]
    type = "RSYSLOG_TraditionalFileFormat"
    template = '%TIMESTAMP% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n'
    tz = "America/Los_Angeles"

*Example Heka Message*

:Timestamp: 2014-02-10 12:58:58 -0800 PST
:Type: RSYSLOG_TraditionalFileFormat
:Hostname: trink-x230
:Pid: 0
:UUID: e0eef205-0b64-41e8-a307-5772b05e16c1
:Logger: RsyslogInput
:Payload: "imklog 5.8.6, log source = /proc/kmsg started."
:EnvVersion:
:Severity: 7
:Fields:
    | name:"programname" value_string:"kernel"
--]]

local syslog = require "syslog"

local template = read_config("template")
local msg_type = read_config("type")
local hostname_keep = read_config("hostname_keep")

local msg = {
Timestamp   = nil,
Type        = msg_type,
Hostname    = nil,
Payload     = nil,
Pid         = nil,
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

    if fields.syslogtag then
        fields.programname = fields.syslogtag.programname
        msg.Pid = fields.syslogtag.pid
        fields.syslogtag = nil
    end

    if not hostname_keep then
        msg.Hostname = fields.hostname or fields.source
        fields.hostname = nil
        fields.source = nil
    end

    msg.Payload = fields.msg
    fields.msg = nil

    msg.Fields = fields
    if not pcall(inject_message, msg) then return -1 end
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses the syslog logs and extract fields of common programs.

Config:

- hostname_keep (boolean, defaults to false)
    Always preserve the original 'Hostname' field set by Logstreamer's 'hostname' configuration setting.

- rsyslog_template (string)
    The 'template' configuration string from rsyslog.conf.
    http://rsyslog-5-8-6-doc.neocities.org/rsyslog_conf_templates.html
    If you want more flexibility, set this to nil and use MultiDecoder.
    SyslogDecoder uses Payload and Fields[programmname] as input

- tz (string, optional, defaults to UTC)
    If your rsyslog timestamp field in the template does not carry zone offset information, you may set an offset
    to be applied to your events here. Typically this would be used with the "Traditional" rsyslog formats.

    Parsing is done by `Go <http://golang.org/pkg/time/#LoadLocation>`_, supports values of "UTC", "Local",
    or a location name corresponding to a file in the IANA Time Zone database, e.g. "America/New_York".

*Example Heka Configuration*

.. code-block:: ini

    [SyslogDecoder]
    type = 'SandboxDecoder'
    # Default 8MiB is not enough, use 16MiB
    memory_limit = 16777216
    filename = 'lua_decoders/syslog.lua'

    [SyslogDecoder.config]
    type = 'RSYSLOG_TraditionalFileFormat'
    rsyslog_template = '%TIMESTAMP% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n'
    tz = 'Europe/Paris'

*Example Heka Message*

:Timestamp: 2014-01-10 07:04:56 -0800 PST
:Type: RSYSLOG_TraditionalFileFormat
:Hostname: test.example.org
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Logger: SyslogInput
:Payload: 25F2E5E061: to=<george.desantis@test.example.com>, relay=none, delay=0.05, delays=0.05/0/0/0, dsn=2.0.0, status=sent (test.example.com)
:EnvVersion:
:Severity: 7
:Fields:
    | name:"programname" value:"postfix/discard"
    | name:"postfix_queueid" value:"25F2E5E061"
    | name:"postfix_to" value:"george.desantis@test.example.com"
    | name:"postfix_relay" value:"none"
    | name:"postfix_delay" value:0.05
    | name:"postfix_delay_before_qmgr" value:0.05
    | name:"postfix_delay_in_qmgr" value:0
    | name:"postfix_delay_conn_setup" value:0
    | name:"postfix_delay_transmission" value:0
    | name:"postfix_dsn" value:"2.0.0"
    | name:"postfix_status" value:"sent"
--]]

local l = require 'lpeg'
local syslog = require 'syslog'
local syslog_message = require 'syslog_message'
local postfix = require 'postfix'

local rsyslog_template = read_config('rsyslog_template')
local msg_type = read_config('type')
local hostname_keep = read_config('hostname_keep')

local msg = {
    Timestamp   = nil,
    Type        = msg_type,
    Hostname    = nil,
    Payload     = nil,
    Pid         = nil,
    Severity    = nil,
    Fields      = nil
}

local rsyslog_grammar = nil
if rsyslog_template then
    rsyslog_grammar = syslog.build_rsyslog_grammar(rsyslog_template)
    if not rsyslog_grammar then
        error('Unable to parse rsyslog template')
    end
end

local wildcard_grammars = syslog_message.get_wildcard_grammar()

function process_message ()
    local log = read_message('Payload')
    local fields = nil
    if rsyslog_grammar then
        fields = rsyslog_grammar:match(log)
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
            msg.Severity = fields.syslogseverity or fields['syslogseverity-text']
            or fields.syslogpriority or fields['syslogpriority-text']

            fields.syslogseverity = nil
            fields['syslogseverity-text'] = nil
            fields.syslogpriority = nil
            fields['syslogpriority-text'] = nil
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

    else
        msg = {}
        msg.Uuid = read_message('Uuid')
        if msg_type then
            msg.Type = msg_type
        else
            msg.Type = read_message('Type')
        end
        msg.Logger = read_message('Logger')
        msg.Payload = read_message('Payload')
        msg.EnvVersion = read_message('EnvVersion')
        msg.Hostname = read_message('Hostname')
        msg.Timestamp = read_message('Timestamp')
        msg.Severity = read_message('Severity')
        msg.Pid = read_message('Pid')
        fields = read_message('Fields')
    end

    -- Now parses syslog msg
    if fields and fields.programname and msg.Payload then
        local prog_fields = nil
        -- by programmname=<key>
        local prog_grammar = syslog_message.get_prog_grammar(fields.programname)
        if prog_grammar then
            prog_fields = prog_grammar:match(msg.Payload)
        -- by programmname=postfix/*
        elseif not prog_fields then
            prog_fields = postfix.postfix_match(fields.programname, msg.Payload, true)
        end
        -- by programmname=*
        if not prog_fields then
            for grammar_name, grammar in pairs(wildcard_grammars) do
              prog_fields = grammar:match(msg.Payload)
            end
        end
        if prog_fields then
            for k,v in pairs(prog_fields) do
                fields[k] = v
            end
        else
            fields.syslog_match = false
        end
    end

    msg.Fields = fields
    if not pcall(inject_message, msg) then return -1 end
    return 0
end

-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses a payload containing JSON in Systemd Journal format.

Config:

- type (string, optional, default nil):
    Sets the message 'Type' header to the specified value

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example of Systemd Journal JSON format*

.. code-block:: javascript

  {
    "__CURSOR" : "s=553297482b77481ba5df5ce3c6d965cc;i=e0a2;b=f98839b811bf4b61b5aaf649a9199c9b;m=1bbee12d;t=5723df70c9380;x=772c69e8bb11
    "__REALTIME_TIMESTAMP" : "1427432230654848",
    "__MONOTONIC_TIMESTAMP" : "448717101",
    "_BOOT_ID" : "f88839b811bf4b61b5aaf649a9190c9b",
    "PRIORITY" : "6",
    "_UID" : "0",
    "_GID" : "0",
    "_SYSTEMD_SLICE" : "system.slice",
    "_MACHINE_ID" : "14f2d832739a4500ba270174d9a63549",
    "_HOSTNAME" : "localhost.localdomain",
    "_CAP_EFFECTIVE" : "3fffffffff",
    "_TRANSPORT" : "syslog",
    "SYSLOG_FACILITY" : "9",
    "SYSLOG_IDENTIFIER" : "crond",
    "_COMM" : "crond",
    "_EXE" : "/usr/sbin/crond",
    "_CMDLINE" : "/usr/sbin/crond -n",
    "_SYSTEMD_CGROUP" : "/system.slice/crond.service",
    "_SYSTEMD_UNIT" : "crond.service",
    "_SELINUX_CONTEXT" : "system_u:system_r:crond_t:s0-s0:c0.c1023",
    "MESSAGE" : "(CRON) INFO (@reboot jobs will be run at computer's startup.)",
    "SYSLOG_PID" : "3584",
    "_PID" : "3584",
    "_SOURCE_REALTIME_TIMESTAMP" : "1427432230653971"
  }

*Example Heka Configuration*

.. code-block:: ini

  [SystemdJournalInput]
  type = "ProcessInput"
  ticker_interval = 0
  splitter = "TokenSplitter"
  decoder = "SystemdJournalDecoder"
  stdout = true
  stderr = false

    [SystemdJournalInput.command.0]
    bin = "/usr/bin/journalctl"
    args = ["-b", "-l", "-o", "json", "-f"]

  [SystemdJournalDecoder]
  type = "SandboxDecoder"
  filename = "lua_decoders/systemd_journal.lua"

    [SystemdJournalDecoder.config]
    type = "systemd_journal"
    payload_keep = false

--]]

require "cjson"

local msg_type     = read_config("type")
local payload_keep = read_config("payload_keep")

local msg = {
    EnvVersion = 1,
    Pid        = nil,
    Host       = nil,
    Type       = msg_type,
    Payload    = nil,
    Severity   = nil,
    Fields     = nil
}

function process_message()
    local ok, json = pcall(cjson.decode, read_message("Payload"))
    if not ok then
        return -1
    end

    if payload_keep then
        msg.Payload = read_message("Payload")
    else
        msg.Payload = json["MESSAGE"]

        -- Remove original field to avoid duplication
        json["MESSAGE"] = nil
    end

    msg.Pid = json["_PID"]
    msg.Host = json["_HOSTNAME"]
    msg.Severity = json["PRIORITY"]
    msg.Fields = json

    -- Remove original fields to avoid duplication
    json["_PID"] = nil
    json["_HOSTNAME"] = nil
    json["PRIORITY"] = nil
 
    if not pcall(inject_message, msg) then return -1 end
 
    return 0
end

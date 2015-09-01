-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses a payload containing JSON.

Config:

- type (string, optional, default "json"):
    Sets the message 'Type' header to the specified value,
    will be overridden if Type config option is specified.

- payload_keep (bool, optional, default false)
    Whether to preserve the original log line in the message payload.

- map_fields (bool, optional, default false)
    Enables mapping of json fields to heka message fields.

- Uuid (string, optional, default nil)
    string specifying json field to map to message Uuid,
    expects field value to be a string.

- Type (string, optional, default nil)
    string specifying json field to map to to message Type,
    expects field value to be a string.

- Logger (string, optional, default nil)
    string specifying json field to map to message Logger,
    expects field value to be a string.

- Hostname (string, optional, default nil)
    string specifying json field to map to message Hostname,
    expects field value to be a string.

- Severity (string, optional, default nil)
    string specifying json field to map to message Severity,
    expects field value to be numeric.

- EnvVersion (string, optional, default nil)
    string specifying json field to map to message EnvVersion,
    expects field value to be numeric.

- Pid (string, optional, default nil)
    string specifying json field to map to message Pid,
    expects field value to be numeric

- Timestamp (string, optional, default nil)
    string specifying json field to map to message Timestamp,
    expects field value to be unix epoch timestamp in seconds.

.. code-block:: javascript

    {
      "msg": "Start Request",
      "event": "artemis.web.ensure-running",
      "extra": {
        "workspace-id": "cN907xLngi"
      },
      "time": "2015-05-06T20:40:05.509926234Z",
      "severity": 1
    }

*Example Heka Configuration*

.. code-block:: ini

    [ArtemisLogInput]
    type = "LogstreamerInput"
    log_directory = "/srv/artemis/current/logs"
    file_match = 'artemis\.log'
    decoder = "JsonDecoder"

    [JsonDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/json.lua"

        [JsonDecoder.config]
        type = "artemis"
        payload_keep = true
        map_fields = true
        Severity = "severity"

*Example Heka Message*

:Timestamp: 2015-05-06 20:40:05 -0800 PST
:Type: artemis
:Hostname: test.example.com
:Pid: 0
:UUID: 8e414f01-9d7f-4a48-a5e1-ae92e5954df5
:Payload:
:EnvVersion:
:Severity: 1
:Fields:
    | name:"msg" value_type:STRING value_string:"Start Request"
    | name:"event" value_type:STRING value_string:"artemis.web.ensure-running"
    | name:"extra.workspace-id" value_type:STRING value_string:"cN907xLngi"
    | name:"time" value_type:STRING value_string:"2015-05-06T20:40:05.509926234Z"
--]]

require "cjson"
local util = require("util")

local payload_keep = read_config("payload_keep")
local map_fields   = read_config("map_fields")

local field_map = {
    Payload    = read_config("Payload")
    Uuid       = read_config("Uuid"),
    Type       = read_config("Type"),
    Logger     = read_config("Logger"),
    Hostname   = read_config("Hostname"),
    Severity   = read_config("Severity"),
    EnvVersion = read_config("EnvVersion"),
    Pid        = read_config("Pid"),
    Timestamp  = read_config("Timestamp")
}

local field_type_map = {
    Payload    = "string",
    Uuid       = "string",
    Type       = "string",
    Logger     = "string",
    Hostname   = "string",
    Severity   = "number",
    EnvVersion = "number",
    Pid        = "number",
    Timestamp  = "number"
}

local msg = {
    Payload    = nil,
    Uuid       = nil,
    Type       = read_config("type") or "json",
    Logger     = nil,
    Hostname   = nil,
    Severity   = nil,
    EnvVersion = nil,
    Pid        = nil,
    Timestamp  = nil,
    Fields     = nil
}

function process_message()
    local ok, json = pcall(cjson.decode, read_message("Payload"))
    if not ok then
        return -1, "Failed to decode JSON."
    end

    -- keep payload, or not
    if payload_keep then
        msg.Payload = read_message("Payload")
    end

    -- map fields
    if map_fields then
        for F, f in pairs(field_map) do
            if type(json[f]) == field_type_map[F] then
                if F == "Timestamp" then
                    msg[F] = json[f] * 1e9
                else
                    msg[F] = json[f]
                end
                json[f] = nil
            end
        end
    end

    -- flatten and assign remaining fields to heka fields
    local flat = {}
    util.table_to_fields(json, flat, nil)
    msg.Fields = flat

    if not pcall(inject_message, msg) then
      return -1, "Failed to inject message."
    end

    return 0
end

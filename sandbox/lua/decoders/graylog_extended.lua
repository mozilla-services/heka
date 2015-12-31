-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Parses a payload containing JSON in the Graylog2 Extended Format specficiation.
http://graylog2.org/resources/gelf/specification

Config:

- type (string, optional, default nil):
    Sets the message 'Type' header to the specified value

- payload_keep (bool, optional, default false)
    Always preserve the original log line in the message payload.

*Example of Graylog2 Exteded Format Log*

.. code-block:: javascript

  {
    "version": "1.1",
    "host": "rogueethic.com",
    "short_message": "This is a short message to identify what is going on.",
    "full_message": "An entire backtrace\ncould\ngo\nhere",
    "timestamp": 1385053862.3072,
    "level": 1,
    "_user_id": 9001,
    "_some_info": "foo",
    "_some_env_var": "bar"
  }

*Example Heka Configuration*

.. code-block:: ini

    [GELFLogInput]
    type = "LogstreamerInput"
    log_directory = "/var/log"
    file_match = 'application\.gelf'
    decoder = "GraylogDecoder"

    [GraylogDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/graylog_decoder.lua"

        [GraylogDecoder.config]
        type = "gelf"
        payload_keep = true
--]]

require "cjson"

local msg_type     = read_config("type")
local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp  = nil,
    EnvVersion = nil,
    Hostname   = nil,
    Type       = msg_type,
    Payload    = nil,
    Fields     = nil,
    Severity   = nil
}

function process_message()
    local ok, json = pcall(cjson.decode, read_message("Payload"))
    if not ok then
        return -1
    end

    if payload_keep then
        msg.Payload = read_message("Payload")
    end

    if type(json["timestamp"]) ~= "number" then return -1 end
    msg.Timestamp = json["timestamp"] * 1e9

    msg.EnvVersion = json["version"]
    msg.Severity = json["level"]
    msg.Hostname = json["host"]
    msg.Fields = json

    -- Remove original fields to avoid duplication
    json["timestamp"] = nil
    json["version"] = nil
    json["level"] = nil
    json["host"] = nil
 
    if not pcall(inject_message, msg) then return -1 end
 
    return 0
end

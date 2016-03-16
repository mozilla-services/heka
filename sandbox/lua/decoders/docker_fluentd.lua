-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Decode data from Docker's Fluentd logging driver.

**Note**: The Fluentd logging driver is available in Docker 1.8.0rc1 and later.

Config:

- type (string, optional, default nil):
    Sets the message 'Type' header to the specified value

*Example Heka Configuration*

.. code-block:: ini

    [FluentdInput]
    type = "TcpInput"
    address = ":24224"
    splitter = "MessagePackSplitter"
    decoder = "DockerFluentdDecoder"

    [MessagePackSplitter]
    # No config

    [DockerFluentdDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/docker_fluentd.lua"

        [DockerFluentdDecoder.config]
        type = "docker-fluentd"

*Example Heka Message*

.. code-block:: bash

    docker run \
        --log-driver fluentd \
        --log-opt fluentd-address=YOUR_HEKA:24224 \
        -d busybox \
        echo Hello world

This command should generate something like this:

:Timestamp: 2015-08-03 20:41:06 +0000 UTC
:Type: docker-fluentd
:Hostname: 192.168.59.103:60088
:Pid: 0
:Uuid: da45d947-037d-4870-abbe-671d820ebe8d
:Logger: stdout
:Payload: Hello world
:EnvVersion: 
:Severity: 7
:Fields:
    | name:"container_name" type:string value:"/suspicious_meitner"
    | name:"tag" type:string value:"docker.7dc19982364b"
    | name:"container_id" type:string value:"7dc19982364ba459958041d2fe85e8bdc3825d06397296ddd981c51e5f15cb89"

--]]

local mp = require "msgpack"

local msg_type = read_config("type")

local msg = {
    Timestamp  = nil,
    EnvVersion = nil,
--    Hostname   = nil,
    Type       = msg_type,
    Payload    = nil,
    Fields     = nil,
    Severity   = nil
}

-- Unpack and validate Fluentd message pack
function decode(mpac)
    local ok, data = pcall(mp.unpack, mpac)
    if not ok then
        return "MessagePack decode error"
    end
    
    if type(data) ~= "table" then
        return "Wrong format" 
    end

    tag, timestamp, record = unpack(data)

    if type(tag) ~= "string" or type(timestamp) ~= "number" or type(record) ~= "table" then
        return "Wrong format"
    end

    return nil, tag, timestamp, record
end


function process_message()
    err, tag, timestamp, record = decode(read_message("Payload"))
    if err ~= nil then return -1, err end

    msg.Timestamp = timestamp * 1e9
    msg.Payload = record["log"]
    msg.Logger = record["source"]
    record["source"] = nil
    record["log"] = nil

    record["tag"] = tag
    msg.Fields = record

    if not pcall(inject_message, msg) then return -1 end
    return 0
end

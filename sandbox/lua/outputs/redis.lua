-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Pushes message onto a Redis list.
Supports bulk loading and optional json encoding.

Config:

- server (string, optional, default "127.0.0.1")
  Specify the redis host to connect to.

- port (uint, optional, default 6379)
  Specify the port redis is listening on.

- channel (string, optional, default "heka")
  Specify the list key to RPUSH messages to.

- max_messages (uint, optional, default 1)
  Specify the maximum number of messages to buffer for bulk loading.

- flush_interval (uint, optional, default 5)
  Specify the maximum number of seconds between bulk loads.

- encoding (string, optional, default "raw")
  Specify either "raw" or "json" message encoding.

*Example Heka Configuration*

.. code-block:: ini

    [RedisOutput]
    type = "SandboxOutput"
    filename = "lua_outputs/redis.lua"
    module_directory = "/usr/share/heka/lua_modules;/usr/share/heka/lua_io_modules"
    message_matcher = "Type == 'logfile'"
    ticker_interval = 5

    [RedisOutput.config]
    server = "127.0.0.1"
    port = 6379
    channel = "heka-output"
    max_messages = 20
    flush_interval = 10
    encoding = "json"
--]]

-- load modules
local os = require "os"
local string = require "string"
local table = require "table"
local cjson = require "cjson"
local redis = require "redis"

-- set up configuration
local cfg = {
    Server    = read_config("server") or "127.0.0.1",
    Port      = read_config("port") or 6379,
    Channel   = read_config("channel") or "heka",
    MaxMsgs   = read_config("max_messages") or 1,
    MaxTime   = read_config("flush_interval") or 5,
    Encoding  = read_config("encoding") or "raw",
}

-- validate configuration
assert(cfg.MaxMsgs > 0, "max_messages must be greater than zero")
assert(cfg.MaxTime > 0, "flush_interval must be greater than zero")

-- normalize time to ns
cfg.MaxTime = cfg.MaxTime * 1e9

-- track state
local msg = ""
local count = 0
local last_flush = 0

-- persist msg buffer
msg_buffer = {}

-- establish redis connection
local ok, client = pcall(function()
  return redis.connect(cfg.Server, cfg.Port)
end)

assert(ok, "Redis connection must be successful.")

function bulk_load()
    -- load messages
    unpacked = unpack(msg_buffer)

    if unpacked then
        local _, ok = pcall(function()
            return client:rpush(cfg.Channel, unpacked)
        end)

        if ok then
            msg_buffer = {}; count = 0; last_flush = os.time() * 1e9
        else
            error("failed to load messages")
        end
    end
end

function process_message()
    local msg = read_message("raw")

    if cfg.Encoding == "json" then
        msg = cjson.encode(decode_message(msg))
    end

    table.insert(msg_buffer, msg)

    count = count + 1; msg = ""

    if count >= cfg.MaxMsgs then
        if not pcall(bulk_load) then
            return -1, "Error loading messages."
        end
    end

    return 0
end

function timer_event(ns)
    if not client:ping() then
        error("Redis not responding.")
    end

    if ns - last_flush >= cfg.MaxTime then
        bulk_load()
    end
end

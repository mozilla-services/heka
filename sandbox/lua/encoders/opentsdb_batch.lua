-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Extracts Field data from messages and generates some OpenTSDB-compliant JSON
(http://opentsdb.net/docs/build/html/api_http/put.html).

Can optionally buffer and flush events after a configurable number of messages
(flush_count) or after a configurable number of seconds have passed between the
last Timestamp and the current one (flush_delta) (encoders currently have no
support for timer_event()s)

Config:

- flush_count (number, default 1)
    Flush the buffer every 'flush_count' messages.

- flush_delta (number, default 0)
    Flush the buffer if 'flush_delta' seconds have elapsed since the last
    Timestamp and the current one.

- tag_prefix (string, optional, default "")
    Convert Fields to OpenTSDB tags if they match the tag_prefix.
    (tag_prefix is stripped from the tag name)

- tag_list (string, optional, default "")
    A list of fields (separated by spaces) which will be converted into tags.
    You can use either "tag_list", "tag_prefix" or both.

- skip_fields (string, optional, default "")
    A list of fields (separated by spaces) which will be dropped from processing.

- type_as_prefix (boolean, optional, default false)
    Prepend metrics when encoding them to OpenTSDB format by Type field.

- ts_from_message (boolean, optional, default true)
    Use the Timestamp field (otherwise "now")

- add_hostname_if_missing (boolean, optional, default false)
    If no 'host' tag has been seen, append one with the value of the
    Hostname field.


*Example Heka Configuration*
.. code-block:: ini

    [OpentsdbEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/opentsdb_batch.lua"

      [OpentsdbEncoder.config]
      ts_from_message = true
      add_hostname_if_missing = true
      type_as_prefix = true
      tag_prefix = "tag_"
      tag_list = "env dc"
      skip_fields = "Pid"

    [opentsdb]
    type = "HttpOutput"
    message_matcher = "Fields[MetricOpentsdb] != NIL"
    address = "http://opentsdb.example.com:4242/api/put"
    encoder = "OpentsdbEncoder"

*Example Output*
.. code-block:: json
    [{"value":0.05,"timestamp":1443535912,"metric":"stats.loadavg.15MinAvg","tags":{"env":"test","host":"example.com"}},{"value":2,"timestamp":1443535912,"metric":"stats.loadavg.NumProcesses","tags":{"env":"test","host":"example.com"}},{"value":0.01,"timestamp":1443535912,"metric":"stats.loadavg.5MinAvg","tags":{"env":"test","host":"example.com"}},{"value":0,"timestamp":1443535912,"metric":"stats.loadavg.1MinAvg","tags":{"env":"test","host":"example.com"}}]

--]=]
_PRESERVATION_VERSION = read_config("preservation_version") or 0
_PRESERVATION_VERSION = _PRESERVATION_VERSION + 1 -- force a revision update due to internal changes

require "cjson"
require "table"
require "string"
require "os"

local math = require "math"
local l = require "lpeg"
l.locale(l)

local flush_count     = read_config("flush_count") or 1
local flush_delta     = read_config("flush_delta") or 0
local tag_prefix      = read_config("tag_prefix") or error("`tag_prefix` setting required")
local tag_list_str    = read_config("tag_list") or ""
local skip_fields_str = read_config("skip_fields") or ""
local type_as_prefix  = read_config("type_as_prefix")
local ts_from_message = read_config("ts_from_message")
local add_hostname    = read_config("add_hostname_if_missing")

if ts_from_message == nil then
  ts_from_message = true
end

tag_list = {}
for field in tag_list_str:gmatch("[%S]+") do
  tag_list[field] = true
end

skip_fields = {}
for field in skip_fields_str:gmatch("[%S]+") do
  skip_fields[field] = true
end

local prefix = l.P(tag_prefix)
local chars = l.alnum + l.S("_-./")
local grammar = prefix * l.C(l.alpha * chars^0)

buffer = {}
buffered_messages = 0
last_flush = 0

function flush()
  add_to_payload(cjson.encode(buffer))
  inject_payload()
  last_flush = os.time()
  buffer = {}
  buffered_messages = 0
end


function process_message()

  local ts
  if ts_from_message then
    ts = math.floor(read_message("Timestamp") / 1e9)
  else
    ts = os.time()
  end

  local typ=read_message("Type")
  local tags = {}
  local metrics = {}

  local msg = decode_message(read_message("raw"))
  if not msg.Fields then
    return -1, "Malformed message (no fields)"
  end

  for _, field in ipairs(msg.Fields) do
    local name = field.name      -- these are related to way Heka works
    local value = field.value[1] --

    local tag = grammar:match(name) or (tag_list[name] and name)
    if tag then
      tags[tag] = value
    else
      value = tonumber(value)
      if value and not skip_fields[name] then
        if type_as_prefix then
          name = string.format("%s.%s", typ, name)
        end
        metrics[name] = value
      end
    end
  end

  if not tags.host and add_hostname then
    tags.host = msg.Hostname
  end

  for name, value in pairs(metrics) do
    local message = {}
    message.metric = name
    message.value = value
    message.timestamp = ts
    message.tags = tags
    buffered_messages = buffered_messages + 1
    buffer[buffered_messages] = message
  end

  -- flush the buffer
  if (flush_count > 0 and buffered_messages >= flush_count) or
     (flush_delta > 0 and os.time() - last_flush > flush_delta) then
    flush()
  else
    return -2
  end

  return 0
end

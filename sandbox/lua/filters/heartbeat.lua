--[[
Heartbeat monitoring per host.

Generates a JSON structure that can be used to create a custom heartbeat dashboard.
The output consists of a row per host which includes the host's 
last_heartbeat, last_alert and status.

This plugin also sends an alert when the heartbeat_timeout is exceeded and
supports alert throttling to reduce noise.

Config:

- heartbeat_timeout (uint, optional, default 30)
    Sets the maximum duration (in seconds) between heartbeats before
    an alert is sent.

- alert_throttle (uint, optional, default 300)
    Sets the minimum duration (in seconds) between alert event outputs.

*Example Heka Configuration*

.. code-block:: ini

    [heartbeat]
    type = "SandboxFilter"
    filename = "lua_filters/heartbeat.lua"
    ticker_interval = 30
    preserve_data = true
    message_matcher = "Type == 'heartbeat'"

      [heartbeat.config]
      heartbeat_timeout = 30
      alert_throttle = 300
      
    [alert-encoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/alert.lua"
    
    [email-alert]
    type = "SmtpOutput"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'alert'"
    send_from = "acme-alert@example.com"
    send_to = ["admin@example.com"]
    auth = "Plain"
    user = "smtp-user"
    password = "smtp-pass"
    host = "mail.example.com:25"
    encoder = "alert"

*Example Output*

.. code-block:: json

  {"ip-10-0-0-11":{"last_heartbeat":1415311858257,"status":"up","last_alert":1415310603648},"ip-10-0-0-187":{"last_heartbeat":1415311856214,"status":"down","last_alert":1415310603648}

:Timestamp: 2014-05-14T14:20:18Z
:Hostname: ip-10-226-204-51
:Plugin: heartbeat
:Alert: Missing Heartbeat - ip-10-0-0-187

--]]

require "cjson"
require "math"
require "string"

local alert = require "alert"
alert.set_throttle(0) -- disable alert module's built-in throttling

local heartbeat_timeout = read_config("heartbeat_timeout") or 30
local alert_throttle = read_config("alert_throttle") or 300

hosts = {}
local floor = math.floor

function process_message()
  local timestamp = floor(read_message("Timestamp") / 1e6) -- in ms
  local hostname = read_message("Hostname")

  local host = hosts[hostname]
  if host then
    host.last_heartbeat = timestamp
  else
    host = {last_heartbeat = timestamp, last_alert = 0, status = "up"}
    hosts[hostname] = host
  end
  return 0
end

function timer_event(ns)
  local current_time = floor(ns / 1e6) -- in ms
  for host, hostinfo in pairs(hosts) do
    if current_time - hostinfo.last_heartbeat > heartbeat_timeout * 1000 then
      if current_time - hostinfo.last_alert > alert_throttle * 1000 then
        alert.queue(ns, string.format("Missing Heartbeat - %s\n", host))
        hostinfo.last_alert = current_time
      end
      hostinfo.status = "down"
    else
      hostinfo.status = "up"
    end
  end
  alert.send_queue(ns)
  inject_payload("json", "heartbeat", cjson.encode(hosts))
end

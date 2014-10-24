-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Produces more human readable alert messages.

Config:

<none>

*Example Heka Configuration*

.. code-block:: ini

    [FxaAlert]
    type = "SmtpOutput"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'alert' && Logger =~ /^Fxa/" || Type == 'heka.sandbox-terminated' && Fields[plugin] =~ /^Fxa/"
    send_from = "heka@example.com"
    send_to = ["alert@example.com"]
    auth = "Plain"
    user = "test"
    password = "testpw"
    host = "localhost:25"
    encoder = "AlertEncoder"

    [AlertEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/alert.lua"

*Example Output*

:Timestamp: 2014-05-14T14:20:18Z
:Hostname: ip-10-226-204-51
:Plugin: FxaBrowserIdHTTPStatus
:Alert: HTTP Status - algorithm: roc col: 1 msg: detected anomaly, standard deviation exceeds 1.5
--]]

require "os"
require "string"

function process_message ()
    local ts = os.date("%FT%TZ", read_message("Timestamp") / 1e9)
    local hn = read_message("Hostname")
    local pi = read_message("Logger")
    local pl = read_message("Payload")

    inject_payload("txt", "",
                   string.format("Timestamp: %s\nHostname: %s\nPlugin: %s\nAlert: %s\n", ts, hn, pi, pl))
    return 0
end


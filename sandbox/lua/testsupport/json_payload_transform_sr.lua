-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "cjson"
require "string"
-- sample input {"name":"android_app_created","created_at":"2013-11-15 22:37:34.709739275"}

local date_pattern = '("created_at":)"(%d+-%d+-%d+) (%d+:%d+:%d+%.%d+)"'

function process_message ()
    local pl = read_message("Payload")
    local json, cnt =  string.gsub(pl, date_pattern, '%1"%2T%3Z"', 1)
    if cnt == 0 then
        return -1
    end

    inject_payload("json", "transformed timestamp S&R", cjson.encode(json))
    return 0
end

-- sample output
--2013/11/18 09:20:41 <
--      Timestamp: 2013-11-18 09:20:41.252096692 -0800 PST
--      Type: logfile
--      Hostname: trink-x230
--      Pid: 0
--      UUID: e8298865-fbc2-422c-873e-1210ce8efd9f
--      Logger: test.log
--      Payload: {"name":"android_app_created","created_at":"2013-11-15T22:37:34.709739275Z"}
--
--      EnvVersion: 0.8
--      Severity: 0
--      Fields: [name:"payload_type" value_type:STRING representation:"file-extension" value_string:"json"  name:"payload_name" value_type:STRING representation:"" value_string:"transformed timestamp" ]



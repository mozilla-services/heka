-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "string"
require "cjson"

-- sample input {"name":"android_app_created","created_at":"2013-11-15 22:37:34.709739275"}

local date_pattern = '^(%d+-%d+-%d+) (%d+:%d+:%d+%.%d+)'
function process_message ()
    local ok, json = pcall(cjson.decode, read_message("Payload"))
    if not ok then
        return -1
    end
    local d, t = string.match(json.created_at, date_pattern)
    if d then
        json.created_at = string.format("%sT%sZ", d, t)
        inject_payload("json", "transformed timestamp", cjson.encode(json))
    end
    return 0
end

-- sample output
--2013/11/15 15:25:56 <
--      Timestamp: 2013-11-15 15:25:56.826184879 -0800 PST
--      Type: logfile
--      Hostname: trink-x230
--      Pid: 0
--      UUID: ef5de908-822a-4fe1-a564-ad3b5a9631c6
--      Logger: test.log
--      Payload: {"name":"android_app_created","created_at":"2013-11-15T22:37:34.709739275Z"}
--
--      EnvVersion: 0.8
--      Severity: 0
--      Fields: [name:"payload_type" value_type:STRING representation:"file-extension" value_string:"json"  name:"payload_name" value_type:STRING representation:"" value_string:"transformed timestamp" ]


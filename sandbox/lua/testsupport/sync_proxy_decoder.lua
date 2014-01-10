-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "string"
require "os"

-- sample log line
-- 1.1.1.4 scl2-sync565.services.mozilla.com user3 [18/Apr/2013:14:00:28 -0700] "POST /1.1/user3/storage/tabs HTTP/1.1" 200 280 "-" "Firefox/20.0.1 FxSync/1.22.0.20130409194949.desktop" "-" "ssl: SSL_RSA_WITH_RC4_128_SHA, version=TLSv1, bits=128" node_s:0.019961 req_s:0.019961 retries:0 req_b:1265 "c_l:760"
local log_pattern = '^(%S-) %S- (%S-) %[(.-)%] "(%S-) (.-) .-" (%d+) (%d+) "(.-)" "(.-)" ".-" ".-" node_s:.- req_s:(%d+%.%d+) retries:%d+ req_b:(%d+) ".-"'

-- sample date 18/Apr/2013:14:00:28 -0700
local offset_pattern = "([+-])(%d%d)(%d%d)"
local date_pattern = '^(%d+)/(%w+)/(%d+):(%d+):(%d+):(%d+) ' .. offset_pattern
local months = {Jan=1, Feb=2, Mar=3, Apr=4, May=5, Jun=6, Jul=7, Aug=8, Sep=9, Oct=10, Nov=11, Dec=12}
local t = {year="", month=1, month_name="", day="", hour="", min="", sec="", offset_sign="", offset_hour="", offset_min=""}

local fields = {
    RemoteIP = {value = "", representation="ipv4"},
    User = "",
    Method = "",
    Url = {value = "", representation="uri"},
    StatusCode = "",
    RequestSize = {value="", representation="B"},
    Referer = "",
    Browser = "",
    ResponseTime = {value="", representation="s"},
    ResponseSize = {value="", representatien="B"}
}

local msg = {
Timestamp = 0,
Type = "SyncProxyLog",
Fields = fields
}

local ds = ""

local function convert_date(date) -- returns nanoseconds since unix epoch
    t.day, t.month_name, t.year, t.hour, t.min, t.sec, t.offset_sign, t.offset_hour, t.offset_min = date:match(date_pattern)
    if not t.day then return nil end

    t.month = months[t.month_name]
    if t.month then
        local offset = 0
        if t.offset_hour then
            offset = (tonumber(t.offset_hour) * 60 * 60) + (tonumber(t.offset_min) * 60)
            if t.offset_sign == "+" then offset = offset * -1 end
        end
        -- for this to work properly the Heka TZ must be UTC
        return (os.time(t) + offset) * 1e9
    end
    return nil
end

function process_message ()    
    local log = read_message("Payload")
    fields.RemoteIP.value,
    fields.User,
    ds,
    fields.Method,
    fields.Url.value,
    fields.StatusCode,
    fields.RequestSize.value,
    fields.Referer,
    fields.Browser,
    fields.ResponseTime.value,
    fields.ResponseSize.value = log:match(log_pattern)
    if not ds then return -1 end
    msg.Timestamp = convert_date(ds)
    if msg.Timestamp == nil then return -1 end
    inject_message(msg)
    return 0
end

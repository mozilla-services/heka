-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local types = {
    UNKNOWN         = 1,
    USER_AUTH       = 2,
    USER_ACCT       = 3,
    LOGIN           = 4,
    USER_START      = 5,
    USER_END        = 6,
    USER_LOGIN      = 7,
    USER_LOGOUT     = 8,
    CRED_ACQ        = 9,
    CRED_REF        = 10,
    CRYPTO_KEY_USER = 11,
    CRYPTO_SESSION  = 12,
    ADD_USER        = 13, 
    ADD_GROUP       = 14
    }  

data = circular_buffer.new(720, 14, 60)
for k,v in pairs(types) do
    data:set_header(v,k)
end

function process_message ()
    local ts = read_message("Fields[AuditTime]")
    if ts == nil then return 0 end
    ts = tonumber(ts) * 1e9

    local type = read_message("Fields[AuditType]")
    local id = types[type]
    if id == nil then 
        id = types.UNKNOWN
    end
    data:add(ts, id, 1)
    return 0
end

function timer_event(ns)
    output(data)
    inject_message("cbuf", "Counts by type")
end


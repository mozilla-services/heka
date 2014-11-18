-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require("lpeg")
local l = lpeg
l.locale(l)

local space = l.space^1

local timestamp = l.Cg((l.R"09"^1 / "%0000000000"), "Timestamp")

local severity = l.Cg(
    l.P"debug"               /"7"
    + l.P"info"              /"6"
    + l.P"notice"            /"5"
    + (l.P"warning" + "warn")/"4"
    + (l.P"error" + "err")   /"3"
    + l.P"crit"              /"2"
    + l.P"alert"             /"1"
    + (l.P"emerg" + "panic") /"0"
    , "Severity")

local key = l.C(l.alpha^1)

local value = l.C(l.R"!~"^1)

local pair = space * l.Cg(key * "=" * value)

local fields = l.Cg(l.Cf(l.Ct("") * pair^0, rawset), "Fields")

local grammar = l.Ct(timestamp * space * severity * fields)

function process_message ()
    local payload = read_message("Payload")
    local t = grammar:match(payload)
    if t then
        inject_message(t)
    else
        return -1, "LPeg grammar failed to match"
    end
    return 0
end

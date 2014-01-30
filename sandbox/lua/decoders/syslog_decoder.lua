-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

-- Original grok filters
-- POSINT \b(?:[0-9]+)\b
-- GREEDYDATA .*
-- HOUR (?:2[0123]|[01][0-9])
-- MINUTE (?:[0-5][0-9])
-- SECOND (?:(?:[0-5][0-9]|60)(?:[.,][0-9]+)?)
-- TIME (?!<[0-9])%{HOUR}:%{MINUTE}(?::%{SECOND})(?![0-9])
-- MONTH \b(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\b
-- MONTHDAY (?:3[01]|[1-2]?[0-9]|0?[1-9])
-- SYSLOGTIMESTAMP %{MONTH} +%{MONTHDAY} %{TIME}
-- SYSLOGFACILITY <%{POSINT:facility}.%{POSINT:priority}>
-- IP (?<![0-9])(?:(?:25[0-5]|2[0-4][0-9]|[0-1]?[0-9]{1,2})[.](?:25[0-5]|2[0-4][0-9]|[0-1]?[0-9]{1,2})[.](?:25[0-5]|2[0-4][0-9]|[0-1]?[0-9]{1,2})[.](?:25[0-5]|2[0-4][0-9]|[0-1]?[0-9]{1,2}))(?![0-9])
-- HOSTNAME \b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)
-- IPORHOST (?:%{HOSTNAME}|%{IP})
-- SYSLOGHOST %{IPORHOST}
-- PROG (?:[\w._/-]+)
-- SYSLOGPROG %{PROG:program}(?:\[%{POSINT:pid}\])?
-- SYSLOGBASE %{SYSLOGTIMESTAMP:timestamp} (?:%{SYSLOGFACILITY} )?%{SYSLOGHOST:logsource} %{SYSLOGPROG}:

require("lpeg")

function addToSet(set, key)
    set[key] = true
end

function removeFromSet(set, key)
    set[key] = nil
end

function setContains(set, key)
    return set[key] ~= nil
end


local l = lpeg
l.locale(l)

local POSINT = l.R"09"^1
local GREEDYDATA = l.P(1)^0
local HOUR = (l.P"2" * l.R"03") + (l.R"01" * l.R"09") + (l.R"09")
local MINUTE = l.R"05" * l.R"09"
local SECOND =((l.R"05" * l.R"09" )+l.P"60") * (((l.P"."+l.P",")*l.R"09"^1)^-1)
local TIME = HOUR * l.P":" * MINUTE * (l.P":" * SECOND)^-1
local MONTH = ((l.P"Jan" * l.P"uary"^-1) + (l.P"Feb" * l.P"ruary"^-1) + (l.P"Mar" * l.P"ch"^-1) + (l.P"Apr" * l.P"il"^-1) + (l.P"May" * l.P""^-1) + (l.P"Jun" * l.P"e"^-1) + (l.P"Jul" * l.P"y"^-1) + (l.P"Aug" * l.P"ust"^-1) + (l.P"Sep" * l.P"tember"^-1) + (l.P"Oct" * l.P"ober"^-1) + (l.P"Nov" * l.P"ember"^-1) + (l.P"Dec" * l.P"embear"^-1))
local MONTHDAY = ((l.P"3" * l.R"01") + (l.R"12" * l.R"09") + (l.P"0" * l.R"19"))
local SYSLOGTIMESTAMP = MONTH * l.P" " * MONTHDAY * l.P" " * TIME
local SYSLOGFACILITY = l.Cg(POSINT, "facility") * "." * l.Cg(POSINT, "priority")
local IP = l.R"09"^3 * (("." * l.R"09"^3)^3)^-1
local HOST_LABEL = (l.R"09" + l.R"az" + l.R"AZ" + l.S"-")^-63
local HOSTNAME = -l.S"-" * HOST_LABEL * ("." * HOST_LABEL)^0
local IPORHOST = HOSTNAME + IP
local PROG = (l.alpha + l.S" ._/-")^1
local SYSLOGPROG = l.Cg(PROG, "program") * l.P"[" * l.Cg(POSINT, "pid") * l.P"]"

-- We need to build up an RFC3339 compatible version of date parsing
local month_int = l.R "09" * l.R"09"
local day_int = l.R "09" * l.R"09"
local TZ_UTC = l.P"Z"
local TZ_OFFSET = (l.S"+-"^-1 * HOUR * ":" * MINUTE)
local TZ = TZ_UTC + TZ_OFFSET
local year_int = l.R"09"* l.R"09"* l.R"09"* l.R"09"
local RFC_3339 = l.Cg(year_int * "-" * month_int * "-" * day_int * "T" * HOUR * l.P":" * MINUTE * l.P":" * SECOND * (TZ)^-1, "syslog_rfc3339")
---


local SYSLOGBASE = (l.Cg(SYSLOGTIMESTAMP, "syslog_timestamp") + RFC_3339) * l.P" " * (SYSLOGFACILITY * l.P" ")^-1 * l.Cg(IPORHOST, "logsource") * l.P" " * SYSLOGPROG 

-- Some systems encode the syslog_priority as a number
-- others encode it as a string.  Tag the fields out as separate names
local SYSLOG_PRI = (l.P"<" * l.Cg(POSINT, "syslog_pri")* l.P">")^-1
local SYSLOG_STR_PRI = (l.P" <" * l.Cg(l.alpha^1, "syslog_str_pri") * l.P">")^-1

local SYSLOG_MESSAGE = SYSLOG_PRI * l.P" "^-1 * SYSLOGBASE * SYSLOG_STR_PRI * l.P": " * l.Cg(GREEDYDATA, "syslog_message")
local grammar = l.Ct(SYSLOG_MESSAGE)


function decode(payload)
    local keyset = {}
    local captures = grammar:match(payload)
    local t = {}
    t["Payload"] = payload

    if captures == nil then
        -- Return the empty table if parsing went badly
        return nil
    end

    for k, v in pairs(captures) do
        addToSet(keyset, k)
    end

    if setContains(keyset, 'pid') then
        t['Pid'] = tonumber(captures['pid'])
        removeFromSet(captures, "pid")
    else
        t['Pid'] = 0
    end

    if setContains(keyset, "syslog_timestamp") then
        -- Need to convert unix ctime to nanoseconds
        captures['syslog_ts'] = captures['syslog_timestamp']
        removeFromSet(captures, "syslog_timestamp")
    elseif setContains(keyset, "syslog_rfc3339") then
        captures['syslog_ts'] = captures["syslog_rfc3339"]
        removeFromSet(captures, "syslog_timestamp")
    end

    t["Fields"] = captures
    return t
end


function process_message()
    local payload = read_message("Payload")
    local t = decode(payload)
    if t then
        inject_message(t)
        return 0
    else
        return -1
    end
end

local string = require 'string'
local rfc3339 = require 'rfc3339'
local os = require 'os'
-- local inspect = require('inspect')
local l = require 'lpeg'
l.locale(l)  -- adds locale entries into 'lpeg' table

SP              = l.P" "^0
PRINTUSASCII    = l.R"!~"
NILVALUE        = l.P"-"
OCTET           = l.P(1)
VERSION         = l.R"19" * l.R"09"^-2

PRIVAL          = l.R"09"^-3
PRI             = l.P"<" * l.Cg(PRIVAL, "pri") * l.P">"

HOSTNAME        = PRINTUSASCII^-255 + NILVALUE

APP_NAME        = PRINTUSASCII^-48 + NILVALUE
PROCID          = PRINTUSASCII^-128 + NILVALUE
DATE_FULLYEAR   = l.R"09" * l.R"09" * l.R"09" * l.R"09"
DATE_MONTH      = l.R"01" * l.R"09"
DATE_MDAY       = l.R"03" * l.R"09"
FULL_DATE       = l.Cg(DATE_FULLYEAR, "year") * l.P"-" * l.Cg(DATE_MONTH, "month") * l.P"-" * l.Cg(DATE_MDAY, "day")


TIME_SECOND     = l.R"05" * l.R"09"
TIME_SECFRAC    = l.P"." * l.R"09"^-6
TIME_MINUTE     = l.R"05" * l.R"09"
TIME_HOUR       = l.R"05" * l.R"09"
TIME_NUMOFFSET  = l.S"+-" * TIME_HOUR * l.P":" * TIME_MINUTE
TIME_OFFSET     = "Z" + TIME_NUMOFFSET
PARTIAL_TIME    = TIME_HOUR * l.P":" * TIME_MINUTE * l.P":" * TIME_SECOND * TIME_SECFRAC^-1
FULL_TIME       = PARTIAL_TIME * TIME_OFFSET

BOM             = l.P(string.char(239, 187, 191))
UTF_8_STRING    = l.alnum^0
PARAM_VALUE     = (l.alnum)^0
MSG             = l.Cg(OCTET^0, "msg")
MSGID           = PRINTUSASCII^-32 + NILVALUE
SD_NAME         =  (l.alpha+l.digit) ^-32 * SP
PARAM_NAME      = SD_NAME
SD_PARAM        = l.Cg(l.C(PARAM_NAME) * SP * "=" * SP * "\""  * l.C(PARAM_VALUE) * "\"")
SD_ID           = l.Cg(l.R"!~"^0, "sd_id")
SD_ELEMENT      = l.P"[" * SD_ID * SP * l.Cg(l.Cf(l.Ct("") * (SP * SD_PARAM)^0, rawset), "sd_params") * SP * l.S"]"
STRUCTURED_DATA = l.Cg(SD_ELEMENT^1, "structured_data") + NILVALUE
DIGIT           = l.R"09"
NILVALUE        = l.P"-"

TIMESTAMP       = FULL_DATE * l.P"T" * FULL_TIME + NILVALUE

HEADER          = PRI * l.Cg(VERSION, "version") * SP * l.Cg(TIMESTAMP, "timestamp") * SP * l.Cg(HOSTNAME, "hostname") * SP * l.Cg(APP_NAME, "app_name") * SP * l.Cg(PROCID, "procid") * SP * l.Cg(MSGID, "msgid")


SYSLOG_MSG      = l.Ct(HEADER * SP * STRUCTURED_DATA * (SP *  MSG)^-1)


function process_message()
    local payload = read_message("Payload")
    local captures = SYSLOG_MSG:match(payload)

    if captures == nil then
        -- Return the empty table if parsing went badly
        return nil
    end

    local t = {}
    t['Timestamp'] = rfc3339.time_ns(rfc3339.grammar:match(captures['timestamp']))
    t['Type'] = 'rfc5424'
    t['Payload'] = paylaod
    t['EnvVersion'] = '0.8'
    t['Pid'] = 0
    t['Severity'] = 6
    t['Fields'] = captures

    inject_message(t)
    return 0
end

local l = require 'lpeg'
l.locale(l)

local num = (l.digit + l.P".")^1
local loadavg = l.Cg(num, "1MinAvg") *
    l.space * l.Cg(num, "5MinAvg") *
    l.space * l.Cg(num, "15MinAvg")
local procs = l.Cg(l.digit^1, "NumProcesses") * l.P"/" * l.digit^1
local latestPid = l.digit^1

local grammar = lpeg.Ct(loadavg * l.space * procs * l.space * latestPid)

local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp = nil,
    Type = "heka.stats.cpustats",
    Payload = nil,
    Fields = nil
}

function process_message()
    local data = read_message("Payload")
    local fields = grammar:match(data)

    if not fields then
        return -1
    end

    msg.Timestamp = fields.time
    fields.time = nil

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields = fields
    inject_message(msg)
    return 0
end

local l = require 'lpeg'
l.locale(l)

local space = l.space^1
local num = l.digit^1

function column(tag)
  return space * l.Cg(num / tonumber, tag)
end

local row = column("ReadsCompleted") * column("ReadsMerged") *
    column("SectorsRead") * column("TimeReading") * column("WritesCompleted") *
    column("WritesMerged") * column("SectorsWritten") * column("TimeWriting") *
    column("NumIOInProgress") * column("TimeDoingIO") *
    column("WeightedTimeDoingIO") * l.P("\n")

local grammar = l.Ct(row^1)

local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp = nil,
    Type = "heka.stats.diskstats",
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
    msg.Fields.TickerInterval = read_message("Fields[TickerInterval]")
    inject_message(msg)
    return 0
end


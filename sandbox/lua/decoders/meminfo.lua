local l = require 'lpeg'
l.locale(l)

local name = l.C((1 - l.P":")^1) * ":"
local value = l.Cg( l.digit^1 / tonumber, "value")
local unit = l.C(l.P"kB")
local repr = l.Cg(l.P"\n" * l.Cc"" + l.space * unit * l.P"\n", "representation")
local line = l.Cg(name * l.space^1 *l.Ct(value * repr))
local grammar = l.Cf(l.Ct("") * line^1, rawset)

local payload_keep = read_config("payload_keep")

local msg = {
    Timestamp = nil,
    Type = "heka.stats.meminfo",
    Payload = nil,
    Fields = nil
}

function process_message()
    local data = read_message("Payload")
    local fields = grammar:match(data)

    if not fields then return -1 end

    msg.Timestamp = fields.time
    fields.time = nil

    if payload_keep then
        msg.Payload = data
    end

    msg.Fields = fields
    inject_message(msg)
    return 0
end



require "io"
require "string"
require "dummy"

function process_message()
    local payload = read_message("Payload")
    if string.find(payload, "^FAILURE") then
        return -1, "failure message"
    elseif string.find(payload, "^USERABORT") then
        return 1, "user abort"
    elseif string.find(payload, "^FATAL") then
        error("fatal error")
    else
        local fh = io.open("output.lua.txt", "w+")
        io.output(fh)
        io.write(read_message("Payload"))
        io.close()
    end
    return 0
end

function timer_event(ns)
    error("fatal error timer_event")
end


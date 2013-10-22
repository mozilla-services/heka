local cj = require("cjson")

function process_message()
    if cj ~= cjson then
        return 1
    end

    local payload = read_message("Payload")
    local value = cjson.decode(payload)
    if "bar" ~= value[2].foo then
        return 2
    end

    return 0
end

function timer_event(ns)

end

local msg = {
    Payload = nil
}

local cnt = 1

function process_message()
    msg.Payload = "line " .. cnt
    cnt = cnt + 1
    if cnt == 3 then
        return -1, "failure message"
    end
    inject_message(msg)
    return 0
end

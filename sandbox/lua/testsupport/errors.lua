data = ""

function process_message ()
    local msg = read_message("Payload")

    if msg == "inject_message() no arg" then
        inject_message()
    elseif msg == "inject_message() incorrect arg type" then
        inject_message(nil)
    elseif msg == "inject_message() incorrect number of args" then
        inject_message(1, 2)
    elseif msg == "output() no arg" then
        output()
    elseif msg == "out of memory" then
        for i=1,500 do
            data = data .. "012345678901234567890123456789010123456789012345678901234567890123456789012345678901234567890123456789"
        end
    elseif msg == "out of instructions" then
        while true do
        end
    elseif msg == "operation on a nil" then
        x = x + 1
    elseif msg == "invalid return" then
        return nil
    elseif msg == "no return" then
        return
    end
    return 0
end

function timer_event()
    return
end

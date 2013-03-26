
count = 0

function process_message ()
    count = count + 1
    output(count)
    inject_message()
    return 0
end

function timer_event(ns)
end


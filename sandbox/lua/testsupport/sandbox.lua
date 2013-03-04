count = 0 

function process_message ()
    count = count + 1
    return 0
end


function timer_event()
    if count ~= 0 then
        output("timer event count=", count)
        count = 0
    end
    return 0
end


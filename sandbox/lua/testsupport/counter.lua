lastTime = os.time() * 1e9
lastCount = 0
count = 0
rate = 0.0
rates = {}

function process_message ()
    count = count + 1
    return 0
end


function timer_event(ns)
    local msgsSent = count - lastCount
    if msgsSent == 0 then return end

    local elapsedTime = ns - lastTime
    if elapsedTime == 0 then return end

    lastCount = count
    lastTime = ns
    rate = msgsSent / (elapsedTime / 1e9)
    rates[#rates+1] = rate
    output(string.format("Got %d messages. %0.2f msg/sec", count, rate))
    inject_message()
    
    local samples = #rates
    if samples == 10 then -- generate a summary every 10 samples
        table.sort(rates)
	     local min = rates[1]
	     local max = rates[samples]
	     local sum = 0
        for i, val in ipairs(rates) do
            sum = sum + val
	     end
        output(string.format("AGG Sum. Min: %0.2f Max: %0.2f Mean: %0.2f", min, max, sum/samples))
        inject_message()
	     rates = {}
    end
end


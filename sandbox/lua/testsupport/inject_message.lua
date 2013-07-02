-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local cbuf = circular_buffer.new(1440, 3, 60)
local simple_table = {value=1}
local metric = {MetricName="example",Timestamp=0,Unit="s",Value=0, 
Dimensions={{Name="d1",Value="v1"}, {Name="d2",Value="v2"}},
StatisticValues={{Maximum=0,Minimum=0,SampleCount=0,Sum= 0},{Maximum=0,Minimum=0,SampleCount=0,Sum=0}}}

function process_message ()
    local msg = read_message("Payload")

    if msg == "lua types" then
        output(simple_table, 1.2, " string ", nil, " ", true, " ", false)
        inject_message()
    elseif msg == "cloudwatch metric" then
        output(metric)
        inject_message()
    elseif msg == "external reference" then
        local a = {x = 1, y = 2}
        local b = {a = a}
        output(b)
        inject_message()
    elseif msg == "array only" then
        local a = {1,2,3}
        output(a)
        inject_message()
    elseif msg == "private keys" then
        local a = {x = 1, _m = 1, _private = {1,2}}
        output(a)
        inject_message()
    elseif msg == "table name" then
        local a = {1,2,3,_name="array"}
        output(a)
        inject_message()
    elseif msg == "global table" then
        output(_G)
        inject_message()
    elseif msg == "special characters" then
        output({['special\tcharacters'] = '"\t\r\n\b\f\\/'})
        inject_message()
    elseif msg == "error internal reference" then
        local a = {x = {1,2,3}, y = {2}}
        a.ir = a.x
        output(a)
        inject_message()
    elseif msg == "error circular reference" then
        local a = {x = 1, y = 2}
        a.self = a
        output(a)
        inject_message()
    elseif msg == "error escape overflow" then
        local escape = "\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n"
        for i=1, 10 do
            escape = escape .. escape
        end
        output({escape = escape})
        inject_message()
    end
    return 0
end

function timer_event(ns)
    output(cbuf)
    inject_message("cbuf", "test")
end


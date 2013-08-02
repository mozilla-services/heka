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
    elseif msg == "message" then
        local msg = {Timestamp = 1e9, Type="type", Logger="logger", Payload="payload", EnvVersion="env_version", Hostname="hostname", Severity=9, }
        inject_message(msg)
    elseif msg == "message field" then
        local msg = {Timestamp = 1e9, Fields = {count=1}}
        inject_message(msg)
    elseif msg == "message field array" then
        local msg = {Timestamp = 1e9, Fields = {counts={2,3,4}}}
        inject_message(msg)
    elseif msg == "message field metadata" then
        local msg = {Timestamp = 1e9, Fields = {count={value=5,representation="count"}}}
        inject_message(msg)
    elseif msg == "message field metadata array" then
        local msg = {Timestamp = 1e9, Fields = {counts={value={6,7,8},representation="count"}}}
        inject_message(msg)
    elseif msg == "message field all types" then
        local msg = {Timestamp = 1e9, Fields = {number=1,numbers={value={1,2,3}, representation="count"},string="string",strings={"s1","s2","s3"}, bool=true, bools={true,false,false}}}
        inject_message(msg)
    elseif msg == "error mis-match field array" then
        local msg = {Timestamp = 1e9, Fields = {counts={2,"ten",4}}}
        inject_message(msg)
    elseif msg == "error nil field" then
        local msg = {Timestamp = 1e9, Fields = {counts={}}}
        inject_message(msg)
    elseif msg == "error nil type arg" then
        inject_message(nil)
    elseif msg == "error nil name arg" then
        inject_message("txt", nil)
    elseif msg == "error incorrect number of args" then
        inject_message("txt", "name", 1)
    elseif msg == "message force memmove" then
        local msg = {Timestamp = 1e9, Fields = {string="0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"}}
        inject_message(msg)
    end
    return 0
end

local output_msg = {Timestamp = 1e9, Type="TEST", Logger="GoSpec", Papload="Test Payload", EnvVersion="0.8", Pid=1234, Hostname="hostname", Severity=6, Fields = {foo="bar"}}

function timer_event(ns)
    if ns == 0 then
        output(cbuf)
        inject_message("cbuf", "test")
    elseif ns == 1 then
        inject_message(output_msg)
    elseif ns == 2 then
        output(output_msg)
        inject_message("json")
    end
end


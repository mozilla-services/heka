.. _lua:

===========
Lua Sandbox
===========

API
---

Functions that must be exposed from the Lua sandbox
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
**int process_message()**
    Called by Heka when a message is available to the sandbox

    *Arguments*
        none

    *Return*
        0 for success, non zero for failure

**timer_event(ns)**
    Called by Heka when the ticker_interval expires

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch

    *Return*
        none

Heka functions that are exposed to the Lua sandbox
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
**read_message(variableName, fieldIndex, arrayIndex)**
    Provides access to the Heka message data

    *Arguments*
        - variableName (string)
            - Uuid
            - Type
            - Logger
            - Payload
            - EnvVersion
            - Hostname
            - Timestamp
            - Severity
            - Pid
            - Fields[_name_]
            - Captures[_name_] see: :ref:`message_matcher`
        - fieldIndex (unsigned) only used in combination with the Fields variableName
            - use to retrieve a specific instance of a repeated field _name_
        - arrayIndex (unsigned) only used in combination with the Fields variableName
            - use to retrieve a specific element out of a field containing an array

    *Return* 
        number, string, bool, nil depending on the type of variable requested

**output(type0, type1, ...typeN)**
    Appends data to the payload buffer

    *Arguments*
        - type (number, string, bool, nil)

    *Return* 
        none

**inject_message()**
    Creates a new Heka message using the contents of the output payload buffer
    and then resets the buffer

    *Arguments* 
        none

    *Return* 
        none


Tutorials
---------

How to create a simple sandbox filter
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
1. Implement the required Heka interface in Lua
::

    function process_message ()
        return 0
    end

    function timer_event(ns)
    end

2. Add the business logic (count the number of 'demo' events per minute)
::

    total = 0 -- preserved between restarts since it is in global scope
    local count = 0 -- local scope so this will not be preserved

    function process_message()
        total= total + 1
        count = count + 1
        return 0
    end

    function timer_event(ns)
        output(string.format("%d messages in the last minute; total=%d", count, total))
        count = 0
        inject_message()
    end

3. Setup the configuration
::

    {
        "name"              : "demo_counter",
        "type"              : "SandboxFilter",
        "message_matcher"   : "Type == 'demo'",
        "ticker_interval"   : 60,
            "sandbox": {
                "type"              : "lua",
                "filename"          : "counter.lua",
                "preserve_data"     : true,
                "memory_limit"      : 32767,
                "instruction_limit" : 100,
                "output_limit"      : 256
            }
    }

4. Extending the business logic (count the number of 'demo' events per minute 
per device)
::

    device_counters = {}

    function process_message()
        local device_name = read_message("Fields[DeviceName]")
        if device_name == nil then
            device_name = "_unknown_"
        end

        local dc = device_counters[device_name]
        if dc == nil then
            dc = {count = 1, total = 1}
            device_counters[device_name] = dc
        else
            dc.count = dc.count + 1
            dc.total = dc.total + 1
        end
        return 0
    end

    function timer_event(ns)
        output("#device_name\tcount\ttotal\n")
        for k, v in pairs(device_counters) do
            output(string.format("%s\t%d\t%d\n", k, v.count, v.total))
            v.count = 0
        end
        inject_message()
    end

5. Depending on the number of devices you will most likely want to update the configuration
::

    "memory_limit"      : 65536,
    "instruction_limit" : 20000,
    "output_limit"      : 64512

.. seealso:: `Lua Reference Manual <http://www.lua.org/manual/5.1/>`_


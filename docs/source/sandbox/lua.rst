.. _lua:

Lua Sandbox
===========

The `Lua` sandbox provides full access to the Lua language in a
sandboxed environment under `hekad` that enforces configurable
restrictions.

.. seealso:: `Lua Reference Manual <http://www.lua.org/manual/5.1/>`_

API
---

Functions that must be exposed from the Lua sandbox
---------------------------------------------------

**int process_message()**
    Called by Heka when a message is available to the sandbox.  The 
    instruction_limit configuration parameter is applied to this function call.

    *Arguments*
        none

    *Return*
        - < 0 for non-fatal failure (increments ProcessMessageFailures)
        - 0 for success
        - > 0 for fatal error (terminates the sandbox)

**timer_event(ns)**
    Called by Heka when the ticker_interval expires.  The instruction_limit 
    configuration parameter is applied to this function call.  This function
    is only required in SandboxFilters (SandboxDecoders do not support timer
    events).

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch

    *Return*
        none

Heka functions that are exposed to the Lua sandbox
---------------------------------------------------

**read_config(variableName)**
    Provides access to the sandbox configuration variables.

    *Arguments*
        - variableName (string)

    *Return*
        number, string, bool, nil depending on the type of variable requested

**read_message(variableName, fieldIndex, arrayIndex)**
    Provides access to the Heka message data.

    *Arguments*
        - variableName (string)
            - raw (accesses the raw MsgBytes in the PipelinePack)
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
        - fieldIndex (unsigned) only used in combination with the Fields variableName
            - use to retrieve a specific instance of a repeated field _name_
        - arrayIndex (unsigned) only used in combination with the Fields variableName
            - use to retrieve a specific element out of a field containing an array

    *Return*
        number, string, bool, nil depending on the type of variable requested

**output(arg0, arg1, ...argN)**
    Appends data to the payload buffer, which cannot exceed the output_limit 
    configuration parameter.

    *Arguments*
        - arg (number, string, bool, nil, table, circular_buffer) Lua variable or literal to be appended the output buffer

    *Return*
        none
    
    *Notes*

        Outputting a Lua table will serialize it to JSON according to the following guidelines/restrictions:
            - Tables cannot contain internal of circular references.
            - Keys starting with an underscore are considered private and will not be serialized.
                - '_name' is a special private key that can be used to specify the the name of the top level JSON object, if not provided the default is 'table'.
            - Arrays only use contiguous numeric keys starting with an index of 1. Private keys are the exception i.e. local a = {1,2,3,_name="my_name"} will be serialized as: ``{"my_name":[1,2,3]}\n``
            - Hashes only use string keys (numeric keys will not be quoted and the JSON output will be invalid). Note: the hash keys are output in an arbitrary order i.e. local a = {x = 1, y = 2} will be serialized as: ``{"table":{"y":2,"x":1}}\n``.

        In most cases circular buffers should be directly output using inject_message.  However, in order to create graph annotations the annotation table has to be written to the output buffer followed by the circular buffer.  The output function is the only way to combine this data before injection (use a unique payload_type when injecting a message with a non-standard circular buffer mashups).

**inject_message(payload_type, payload_name)**
    Creates a new Heka message using the contents of the output payload buffer
    and then clears the buffer. Two pieces of optional metadata are allowed and
    included as fields in the injected message i.e., Fields[payload_type] == 'csv' 
    Fields[payload_name] == 'Android Usage Statistics'.  The number of messages
    that may be injected by the process_message or timer_event functions are 
    globally controlled by the hekad :ref:`hekad_command_line_options`; if
    these values are exceeded the sandbox will be terminated.

    *Arguments*
        - payload_type (**optional, default "txt"** string) Describes the content type of the injected payload data.
        - payload_name (**optional, default ""** string) Names the content to aid in downstream filtering.

    *Return*
        none

**inject_message(circular_buffer, payload_name)**
    Creates a new Heka message placing the circular buffer output in the message payload (overwriting whatever is in the output buffer).
    The payload_type is set to the circular buffer output format string. i.e., Fields[payload_type] == 'cbuf'.
    The Fields[payload_name] is set to the provided payload_name.  

    *Arguments*
        - circular_buffer (circular_buffer)
        - payload_name (**optional, default ""** string) Names the content to aid in downstream filtering.

    *Return*
        none

    *Notes*
        - injection limits are enforced as described above

**inject_message(message_table)**
    Creates a new Heka protocol buffer message using the contents of the
    specified Lua table (overwriting whatever is in the output buffer).
    Notes about message fields:

    * Timestamp is automatically generated if one is not provided.  Nanosecond since the UNIX epoch is the only valid format.
    * UUID is automatically generated, anything provided by the user is ignored.
    * Hostname and Logger are automatically set by the SandboxFilter and cannot be overridden.
    * Type is prepended with "heka.sandbox." by the SandboxFilter to avoid data confusion/mis-representation.
    * Fields can be represented in multiple forms and support the following primitive types: string, double, bool.  These constructs should be added to the 'Fields' table in the message structure. Note: since the Fields structure is a map and not an array, like the protobuf message, fields cannot be repeated.
        * name=value i.e., foo="bar"; foo=1; foo=true
        * name={array} i.e., foo={"b", "a", "r"}
        * name={object} i.e. foo={value=1, representation="s"}; foo={value={1010, 2200, 1567}, representation="ms"}
            * value (required) may be a single value or an array of values
            * representation (optional) metadata for display and unit management

    *Arguments*
        - message_table A table with the proper message structure.

    *Return*
        none

    *Notes*
        - injection limits are enforced as described above

**require(libraryName)**
    By default only the base library is loaded additional libraries must be explicitly specified.

    *Arguments*
        - libraryName (string)
            - **cjson** loaded the cjson.safe module in a global cjson table, exposing the decoding functions only. http://www.kyne.com.au/~mark/software/lua-cjson-manual.html.
            - **lpeg** loads the Lua Parsing Expression Grammar Library http://www.inf.puc-rio.br/~roberto/lpeg/lpeg.html
            - **math**
            - **os**
            - **string**
            - **table**
            - **circular_buffer** :ref:`circular_buffer`

    *Return*
        a table (which is also globally registered with the library name).

Sample Lua Message Structure
----------------------------
.. code-block:: lua

    {
    Uuid        = "data",               -- always ignored
    Logger      = "nginx",              -- ignored in the SandboxFilter
    Hostname    = "bogus.mozilla.com",  -- ignored in the SandboxFilter

    Timestamp   = 1e9,                   
    Type        = "TEST",               -- will become "heka.sandbox.TEST" in the SandboxFilter
    Papload     = "Test Payload",
    EnvVersion  = "0.8",
    Pid         = 1234, 
    Severity    = 6, 
    Fields      = {
                http_status     = 200, 
                request_size    = {value=1413, representation="B"}
                }
    }

.. _lua_tutorials:

Lua Sandbox Tutorial
====================

How to create a simple sandbox filter
-------------------------------------

1. Implement the required Heka interface in Lua

.. code-block:: lua

    function process_message ()
        return 0
    end

    function timer_event(ns)
    end

2. Add the business logic (count the number of 'demo' events per minute)

.. code-block:: lua

    require "string"

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

.. code-block:: ini

    [demo_counter]
    type = "SandboxFilter"
    message_matcher = "Type == 'demo'"
    ticker_interval = 60
    script_type = "lua"
    filename = "counter.lua"
    preserve_data = true
    memory_limit = 32767
    instruction_limit = 100
    output_limit = 256

4. Extending the business logic (count the number of 'demo' events per minute
per device)

.. code-block:: lua

    require "string"

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

5. Depending on the number of devices being counted you will most likely want to update the configuration to account for the additional resource requirements.

.. code-block:: ini

    memory_limit = 65536
    instruction_limit = 20000
    output_limit = 64512

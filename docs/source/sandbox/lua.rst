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

**int, string process_message()**
    This is the entry point for input plugins to start creating messages. For
    all other plugin types it is called by Heka when a message is available to
    the sandbox. The instruction_limit configuration parameter is applied to
    this function call for non input plugins.

    *Arguments*
        none

    *Return*
        - int
            - < 0 for non-fatal failure (increments ProcessMessageFailures)
            - -2 for no output, but no error (encoders only)
            - 0 for success
            - > 0 for fatal error (terminates the sandbox)
        - string optional error message

**timer_event(ns)**
    Called by Heka when the ticker_interval expires.  The instruction_limit
    configuration parameter is applied to this function call.  This function
    is only required in SandboxFilters that have a ticker_interval configuration
    greater than zero.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch

    *Return*
        none

Core functions that are exposed to the Lua sandbox
--------------------------------------------------
See: https://github.com/mozilla-services/lua_sandbox/

**require(libraryName)**

    *Available In*
        All plugin types

**add_to_payload(arg1, arg2, ...argN)**
    Appends the arguments to the payload buffer for incremental construction of
    the final payload output (inject_payload finalizes the buffer and sends the
    message to Heka).  This function is simply a rename of the generic sandbox
    *output* function to improve the readability of the plugin code.

    *Arguments*
        - arg (number, string, bool, nil, circular_buffer)

    *Return*
        none

    *Available In*
        Decoders, filters, encoders

Heka specific functions that are exposed to the Lua sandbox
-----------------------------------------------------------
**read_config(variableName)**
    Provides access to the sandbox configuration variables.

    *Arguments*
        - variableName (string)

    *Return*
        number, string, bool, nil depending on the type of variable requested

    *Available In*
        All plugin types

**read_message(variableName, fieldIndex, arrayIndex)**
    Provides access to the Heka message data. Note that both `fieldIndex` and
    `arrayIndex` are zero-based (i.e. the first element is 0) as opposed to
    Lua's standard indexing, which is one-based.

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
            - use to retrieve a specific instance of a repeated field _name_;
              zero indexed
        - arrayIndex (unsigned) only used in combination with the Fields variableName
            - use to retrieve a specific element out of a field containing an array; zero
              indexed

    *Return*
        number, string, bool, nil depending on the type of variable requested

    *Available In*
        Decoders, filters, encoders, outputs

.. _write_message:

**write_message(variableName, value, representation, fieldIndex, arrayIndex)**
    .. versionadded:: 0.5

    Mutates specified field value on the message that is being decoded.

    *Arguments*
        - variableName (string)
            - Uuid (accepts raw bytes or RFC4122 string representation)
            - Type (string)
            - Logger (string)
            - Payload (string)
            - EnvVersion (string)
            - Hostname (string)
            - Timestamp (accepts Unix ns-since-epoch number or a handful of
                         parseable string representations.)
            - Severity (number or int-parseable string)
            - Pid (number or int-parseable string)
            - Fields[_name_] (field type determined by value type: bool, number, or string)
        - value (bool, number or string)
            - value to which field should be set
        - representation (string) only used in combination with the Fields variableName
            - representation tag to set
        - fieldIndex (unsigned) only used in combination with the Fields variableName
            - use to set a specfic instance of a repeated field _name_
        - arrayIndex (unsigned) only used in combination with the Fields variableName
            - use to set a specific element of a field containing an array

    *Return*
        none

    *Available In*
        Decoders, encoders

**read_next_field()**
    .. deprecated:: 0.10.0
       Use read_message("raw") instead e.g.,

.. code-block:: lua

    local msg = decode_message(read_message("raw"))
    if msg.Fields then
        for i, f in ipairs(msg.Fields) do
        -- process fields
        end
    end
..

    Iterates through the message fields returning the field contents or nil when the end is reached.

    *Arguments*
        none

    *Return*
        value_type, name, value, representation, count (number of items in the field array)

    *Available In*
        Decoders, filters, encoders, outputs

**inject_payload(payload_type, payload_name, arg3, ..., argN)**

    Creates a new Heka message using the contents of the payload buffer
    (pre-populated with *add_to_payload*) combined with any additional
    payload_args passed to this function.  The output buffer is cleared after
    the injection. The payload_type and payload_name arguments are two pieces of
    optional metadata. If specified, they will be included as fields in the
    injected message e.g., Fields[payload_type] == 'csv',
    Fields[payload_name] == 'Android Usage Statistics'. The number of messages
    that may be injected by the process_message or timer_event functions are
    globally controlled by the hekad :ref:`global configuration options <hekad_global_config_options>`;
    if these values are exceeded the sandbox will be terminated.

    *Arguments*
        - payload_type (**optional, default "txt"** string) Describes the content type of the injected payload data.
        - payload_name (**optional, default ""** string) Names the content to aid in downstream filtering.
        - arg3 (**optional**) Same type restrictions as add_to_payload.
        - ...
        - argN

    *Return*
        none

    *Available In*
        Decoders, filters, encoders

.. _inject_message_message_table:

**inject_message(message)**
    Creates a new Heka protocol buffer message using the contents of the
    specified Lua table (overwriting whatever is in the output buffer).
    Notes about message fields:

    * Timestamp is automatically generated if one is not provided.  Nanosecond since the UNIX epoch is the only valid format.
    * UUID is automatically generated if a 16 byte binary UUID is not provided.
    * Hostname and Logger are automatically set by the SandboxFilter and cannot be overridden.
    * Type is prepended with "heka.sandbox." by the SandboxFilter to avoid data confusion/mis-representation.
    * Fields (hash structure) can be represented in multiple forms and support the following primitive types: string, double, bool.
      These constructs can be added to the 'Fields' table in the message structure.

        * name=value e.g., foo="bar"; foo=1; foo=true
        * name={array} e.g., foo={"b", "a", "r"}
        * name={object} e.g., foo={value=1, representation="s"}; foo={value={1010, 2200, 1567}, value_type=2, representation="ms"}

            * value (required) may be a single value or an array of values
            * value_type (optional) `value_type enum <https://github.com/mozilla-services/heka/blob/dev/message/message.proto#L23>`_.
              This is most useful for specifying that numbers should be treated as integers as opposed defaulting to doubles.
            * representation (optional) metadata for display and unit management

    * Fields (array structure)
        * same as above but the hash key name is moved into the object as 'name' e.g., Fields = {{name="foo", value="bar"}}

    *Arguments*
        - message (table or string) A table with the message structure documented below or a string with a Heka protobuf encoded message.

    *Return*
        none

    *Available In*
        Inputs, decoders, filters, encoders

    *Notes*
        Injection limits are only enforced on filter plugins.
        See ``max_*_inject`` in the :ref:`global configuration options <hekad_global_config_options>`.

**decode_message(heka_protobuf_string)**
    Converts a Heka protobuf encoded message string into a Lua table.

    *Arguments*
        - heka_message (string) Lua variable containing a Heka protobuf encoded message

    *Return*
        - message (table) The array based version of the message structure with
          the value member always being an array (even if there is only a single
          item).  This format makes working with the output more consistent.
          The wide variation in the inject table format is to ease the
          construction of the message especially when using an LPeg grammar
          transformation.

.. _heka_message_table_structure:


Lua Message Hash Based Field Structure
--------------------------------------
.. code-block:: lua

    {
    Uuid        = "data",               -- ignored if not 16 byte raw binary UUID
    Logger      = "nginx",              -- ignored in the SandboxFilter
    Hostname    = "bogus.mozilla.com",  -- ignored in the SandboxFilter

    Timestamp   = 1e9,
    Type        = "TEST",               -- will become "heka.sandbox.TEST" in the SandboxFilter
    Payload     = "Test Payload",
    EnvVersion  = "0.8",
    Pid         = 1234,
    Severity    = 6,
    Fields      = {
                http_status     = 200, -- encoded as a double
                request_size    = {value=1413, value_type=2, representation="B"} -- encoded as an integer
                }
    }

Lua Message Array Based Field Structure
---------------------------------------
.. code-block:: lua

    {
    -- same as above
    Fields      = {
                {name="http_status", value=200}, -- encoded as a double
                {name="request_size", value=1413, value_type=2, representation="B"} -- encoded as an integer
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
        count = 0
        inject_payload("txt", "",
                       string.format("%d messages in the last minute; total=%d", count, total))
    end

3. Setup the configuration

.. code-block:: ini

    [demo_counter]
    type = "SandboxFilter"
    message_matcher = "Type == 'demo'"
    ticker_interval = 60
    filename = "counter.lua"
    preserve_data = true

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
        add_to_payload("#device_name\tcount\ttotal\n")
        for k, v in pairs(device_counters) do
            add_to_payload(string.format("%s\t%d\t%d\n", k, v.count, v.total))
            v.count = 0
        end
        inject_payload()
    end

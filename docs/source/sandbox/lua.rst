.. _lua:

===========
Lua Sandbox
===========

The `Lua` sandbox provides full access to the Lua language in a
sandboxed environment under `hekad` that enforces configurable
restrictions.

.. seealso:: `Lua Reference Manual <http://www.lua.org/manual/5.1/>`_

API
===

Functions that must be exposed from the Lua sandbox
---------------------------------------------------

**int process_message()**
    Called by Heka when a message is available to the sandbox.  The 
    instruction_limit configuration parameter is applied to this function call.

    *Arguments*
        none

    *Return*
        0 for success, non zero for failure

**timer_event(ns)**
    Called by Heka when the ticker_interval expires.  The instruction_limit 
    configuration parameter is applied to this function call.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch

    *Return*
        none

Heka functions that are exposed to the Lua sandbox
---------------------------------------------------

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
        - arg (number, string, bool, nil, table) Lua variable or literal to be appended the output buffer

    *Return*
        none
    
    *Notes*
        Outputting a Lua table will serialize it to JSON according to the following guidelines/restrictions:
            - Tables cannot contain internal of circular references.
            - Keys starting with an underscore are considered private and will not be serialized.
                - '_name' is a special private key that can be used to specify the the name of the top level JSON object, if not provided the default is 'table'.
            - Arrays only use contiguous numeric keys starting with an index of 1. Private keys are the exception i.e. local a = {1,2,3,_name="my_name"} will be serialized as: ``{"my_name":[1,2,3]}\n``
            - Hashes only use string keys (numeric keys will not be quoted and the JSON output will be invalid). Note: the hash keys are output in an arbitrary order i.e. local a = {x = 1, y = 2} will be serialized as: ``{"table":{"y":2,"x":1}}\n``.

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

Circular Buffer Library
=======================
The library is a sliding window time series data store and is implemented in
the ``circular_buffer`` table.

Constructor
-----------
circular_buffer.\ **new**\ (rows, columns, seconds_per_row)

    *Arguments*
        - rows (unsigned) The number of rows in the buffer (must be > 1)
        - columns (unsigned)The number of columns in the buffer (must be > 0)
        - seconds_per_row (unsigned) The number of seconds each row represents (must be > 0).

    *Return*
        A circular buffer object.

Methods
-------
.. note::
    All column arguments are 1 based. If the column is out of range for the 
    configured circular buffer a fatal error is generated.

double **add**\ (nanoseconds, column, value)

    *Arguments*
        - nanosecond (unsigned) The number of nanosecond since the UNIX epoch. The value is used to determine which row is being operated on.
        - column (unsigned) The column within the specified row to perform an add operation on.
        - value (double) The value to be added to the specified row/column.

    *Return*
        The value of the updated row/column or nil if the time was outside the range of the buffer.

double **set**\ (nanoseconds, column, value)

    *Arguments*
        - nanosecond (unsigned) The number of nanosecond since the UNIX epoch. The value is used to determine which row is being operated on.
        - column (unsigned) The column within the specified row to perform a set operation on.
        - value (double) The value to be overwritten at the specified row/column.

    *Return*
        The value passed in or nil if the time was outside the range of the buffer.

double **get**\ (nanoseconds, column)

    *Arguments*
        - nanosecond (unsigned) The number of nanosecond since the UNIX epoch. The value is used to determine which row is being operated on.
        - column (unsigned) The column within the specified row to retrieve the data from.

    *Return*
        The value at the specifed row/column or nil if the time was outside the range of the buffer.

int **set_header**\ (column, name, unit, aggregation_method)

    *Arguments*
        - column (unsigned) The column number where the header information is applied.
        - name (string) Descriptive name of the column (maximum 15 characters). Any non alpha numeric characters will be converted to underscores. (default: Column_N)
        - unit (string - optional) The unit of measure (maximum 7 characters). Alpha numeric, '/', and '*' characters are allowed everything else will be converted to underscores. i.e. KiB, Hz, m/s (default: count)
        - aggregation_method (string - optional) Controls how the column data is aggregated when combining multiple circular buffers.
            - **sum** The total is computed for the time/column (default).
            - **min** The smallest value is retained for the time/column.
            - **max** The largest value is retained for the time/column.
            - **avg** The average is computed for the time/column.
            - **none** No aggregation will be performed the column.

    *Return*
        The column number passed into the function.

double **compute**\ (function, column, start, end)

    *Arguments*
        - function (string) The name of the compute function (sum|avg|sd|min|max).
        - column (unsigned) The column that the computation is performed against.
        - start (optional - unsigned) The number of nanosecond since the UNIX epoch. Sets the start time of the computation range; if nil the buffer's start time is used.
        - end (optional- unsigned) The number of nanosecond since the UNIX epoch. Sets the end time of the computation range (inclusive); if nil the buffer's end time is used. The end time must be greater than or equal to the start time.

    *Return*
        The result of the computation for the specifed column over the given range or nil if the range fell outside of the buffer.

Output
------
The circular buffer can be passed to the output() function.  The output will
consist newline delimited rows starting with a json header row followed by the
data rows with tab delimited columns. The time in the header corresponds to the 
time of the first data row, the time for the other rows is calculated using the
seconds_per_row header value.

.. code-block:: txt

    {json header}
    row1_col1\trow1_col2\n
    .
    .
    .
    rowN_col1\trowN_col2\n

Sample Output
-------------
.. code-block:: txt

    {"time":2,"rows":3,"columns":3,"seconds_per_row":60,"column_info":[{"name":"HTTP_200","unit":"count","aggregation":"sum"},{"name":"HTTP_400","unit":"count","aggregation":"sum"},{"name":"HTTP_500","unit":"count","aggregation":"sum"}]}
    10002   0   0
    11323   0   0
    10685   0   0

Example
-------
.. code-block:: lua

    -- This Source Code Form is subject to the terms of the Mozilla Public
    -- License, v. 2.0. If a copy of the MPL was not distributed with this
    -- file, You can obtain one at http://mozilla.org/MPL/2.0/.

    data = circular_buffer.new(1440, 5, 60) -- 1 day at 1 minute resolution
    local HTTP_200      = data:set_header(1, "HTTP_200"     , "count")
    local HTTP_300      = data:set_header(2, "HTTP_300"     , "count")
    local HTTP_400      = data:set_header(3, "HTTP_400"     , "count")
    local HTTP_500      = data:set_header(4, "HTTP_500"     , "count")
    local HTTP_UNKNOWN  = data:set_header(5, "HTTP_UNKNOWN" , "count")

    function process_message()
        local ts = read_message("Timestamp")
        local sc = read_message("Fields[http_status_code]")
        if sc == nil then return 0 end

        if sc >= 200 and sc < 300 then
            data:add(ts, HTTP_200, 1)
        elseif sc >= 300 and sc < 400 then
            data:add(ts, HTTP_300, 1)
        elseif sc >= 400 and sc < 500 then
            data:add(ts, HTTP_400, 1)
        elseif sc >= 500 and sc < 600 then
            data:add(ts, HTTP_500, 1)
        else 
            data:add(ts, HTTP_UNKNOWN, 1)
        end
        return 0
    end

    function timer_event()
        output(data)
        inject_message("cbuf", "HTTP Status Code Statistics")
    end

Setting the inject_message payload_type to "cbuf" will cause the 
:ref:`config_dashboard_output` to automatically generate an HTML page 
containing a graphical view of the data.


.. _lua_tutorials:

Tutorials
=========

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

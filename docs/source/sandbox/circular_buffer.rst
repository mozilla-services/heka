.. _circular_buffer:

Lua Circular Buffer Library
===========================

The library is a sliding window time series data store and is implemented in
the ``circular_buffer`` table.

Constructor
-----------
circular_buffer.\ **new**\ (rows, columns, seconds_per_row, enable_delta)

    *Arguments*
        - rows (unsigned) The number of rows in the buffer (must be > 1)
        - columns (unsigned)The number of columns in the buffer (must be > 0)
        - seconds_per_row (unsigned) The number of seconds each row represents (must be > 0).
        - enable_delta (**optional, default false** bool) When true the changes made to the circular buffer between delta outputs are tracked.

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

cbuf **format**\ (format)
    Sets an internal flag to control the output format of the circular buffer data structure; if deltas are not enabled or there haven't been any modifications, nothing is output.

    *Arguments*
        - format (string)
            - **cbuf** The circular buffer full data set format.
            - **cbufd** The circular buffer delta data set format.

    *Return*
        The circular buffer object.

Output
------
The circular buffer can be passed to the output() function. The output format
can be selected using the format() function.

The cbuf (full data set) output format consists of newline delimited rows
starting with a json header row followed by the data rows with tab delimited
columns. The time in the header corresponds to the time of the first data row,
the time for the other rows is calculated using the seconds_per_row header value.

.. code-block:: txt

    {json header}
    row1_col1\trow1_col2\n
    .
    .
    .
    rowN_col1\trowN_col2\n

The cbufd (delta) output format consists of newline delimited rows starting with
a json header row followed by the data rows with tab delimited columns. The
first column is the timestamp for the row (time_t). The cbufd output will only
contain the rows that have changed and the corresponding delta values for each
column.

.. code-block:: txt

    {json header}
    row14_timestamp\trow14_col1\trow14_col2\n
    row10_timestamp\trow10_col1\trow10_col2\n

Sample Cbuf Output
------------------
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

    require "circular_buffer"

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

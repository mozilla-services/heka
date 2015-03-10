-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Extracts data from SandboxFilter circular buffer output messages and uses it
to generate time series JSON structures that will be accepted by Librato's
`POST API <http://dev.librato.com/v1/post/metrics>`_. It will keep track of
the last time it's seen a particular message, keyed by filter name and output
name. The first time it sees a new message, it will send data from all of the
rows except the last one, which is possibly incomplete. For subsequent
messages, the encoder will automatically extract data from all of the rows
that have elapsed since the last message was received.

The SandboxEncoder `preserve_data` setting should be set to true when using
this encoder, or else the list of received messages will be lost whenever Heka
is restarted, possibly causing the same data rows to be sent to Librato
multiple times.

Config:

- message_key (string, optional, default "%{Logger}:%{payload_name}")
    String to use as the key to differentiate separate cbuf messages from each
    other. Supports :ref:`message field
    interpolation<sandbox_msg_interpolate_module>`.

*Example Heka Configuration*

.. code-block:: ini

    [cbuf_librato_encoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/cbuf_librato.lua"
    preserve_data = true
      [cbuf_librato_encoder.config]
      message_key = "%{Logger}:&{Hostname}:%{payload_name}"

    [librato]
    type = "HttpOutput"
    message_matcher = "Type == 'heka.sandbox-output && Fields[payload_type] == 'cbuf'"
    encoder = "cbuf_librato_encoder"
    address = "https://metrics-api.librato.com/v1/metrics"
    username = "username@example.com"
    password = "SECRET"
        [librato.headers]
        Content-Type = ["application/json"]

*Example Output*

.. code-block:: json

    {"gauges":[{"value":12,"measure_time":1410824950,"name":"HTTP_200","source":"thor"},{"value":1,"measure_time":1410824950,"name":"HTTP_300","source":"thor"},{"value":1,"measure_time":1410824950,"name":"HTTP_400","source":"thor"}]}

--]]

require "cjson"
require "string"
local interp = require "msg_interpolate"
local cbuf_utils = require "cbuf_utils"

last_times = {}

local msg_key_template = read_config("message_key") or "%{Logger}:%{payload_name}"

-- Returns the gauges table, ready to be serialized into JSON. No valid
-- numeric data returns an empty table. Any error returns nil.
function generate_gauges_list(source, headers, rows, idx)
    local col_info = headers.column_info
    if col_info == nil then
        return nil
    end

    -- Calculate time value for first row, then loop through the rows
    -- appending data as we go.
    local time = headers.time + ((idx - 1) * headers.seconds_per_row)
    local gauges = {}
    for i = idx, headers.rows - 1 do -- This omits the last, maybe incomplete, row.
        local label, value, record
        for j, column in ipairs(col_info) do
            value = rows[i][j]
            if value ~= "nan" then
                record = {name=column.name, source=source, value=value, measure_time=time}
                gauges[#gauges+1] = record
            end
        end
        time = time + headers.seconds_per_row
    end
    return gauges
end

function process_message()
    local headers, rows = cbuf_utils.parse_cbuf(read_message("Payload"))
    if not headers then
        return -1
    end

    -- Extract message key and look up last time for the key.
    local hostname = read_message("Hostname")
    local msg_key = interp.interpolate_from_msg(msg_key_template)
    local last_time = last_times[msg_key]
    local start_idx = cbuf_utils.get_start_idx(last_time, headers)
    if start_idx < 0 then
        return start_idx
    end

    -- We've got our starting index, we can generate our gauges and emit the
    -- output.
    local gauges = generate_gauges_list(hostname, headers, rows, start_idx)
    if not gauges then
        return -1
    end
    if #gauges == 0 then
        return -2
    end
    local output = {gauges = gauges}
    inject_payload("json", "output", cjson.encode(output))
    last_times[msg_key] = headers.time
    return 0
end

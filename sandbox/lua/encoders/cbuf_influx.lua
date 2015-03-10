-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

Extracts data from SandboxFilter circular buffer output messages and uses it
to generate JSON structures that will be accepted by InfluxDB's `HTTP API
<http://influxdb.com/docs/v0.8/api/reading_and_writing_data.html>`_. It will
keep track of the last time it's seen a particular message, keyed as specified
by the `message_key` config setting. The first time it sees a message with a
specific key, it will send data from all of the rows except the last one,
which is possibly incomplete. For subsequent messages, the encoder will
automatically extract data from all of the rows that have elapsed since the
last message was received.

The SandboxEncoder `preserve_data` setting should be set to true when using
this encoder, or else the list of received messages will be lost whenever Heka
is restarted, possibly causing the same data rows to be sent multiple times.

Config:

- message_key (string, optional, default "%{Logger}:%{payload_name}")
    String to use as the key to differentiate separate cbuf messages from each
    other. Supports :ref:`message field
    interpolation<sandbox_msg_interpolate_module>`.

- data_name (string, optional, default "%{payload_name}")
    String to use as the "name" value in the generated points data structure.
    Supports :ref:`message field
    interpolation<sandbox_msg_interpolate_module>`.

*Example Heka Configuration*

.. code-block:: ini

    [cbuf_influx_encoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/cbuf_influx.lua"
    preserve_data = true
      [cbuf_influx_encoder.config]
      message_key = "%{Logger}:&{Hostname}:%{payload_name}"

    [influx]
    type = "HttpOutput"
    message_matcher = "Type == 'heka.sandbox-output && Fields[payload_type] == 'cbuf'"
    encoder = "cbuf_influx_encoder"
    address = "http://influxserver.example.com:8086/db/your_series_name/series"
    username = "influx_username"
    password = "influx_password"

*Example Output*

.. code-block:: json


--]]

require "cjson"
require "math"
require "string"
local interp = require "msg_interpolate"
local cbuf_utils = require "cbuf_utils"

last_times = {}

local msg_key_template = read_config("message_key") or "%{Logger}:%{payload_name}"
local data_name_template = read_config("data_name") or "%{payload_name}"

function generate_data(data_name, headers, rows, idx)
    local col_info = headers.column_info
    if col_info == nil then
        return nil
    end

    -- Extract column names.
    num_cols = #col_info
    local col_names = {"time"}
    for i, column in ipairs(col_info) do
        col_names[i+1] = column.name
    end
    -- Calculate time for first row, then loop through the rows, appending
    -- data as we go.
    local points = {}
    local time = headers.time + ((idx-1) * headers.seconds_per_row)
    local pnt_cnt = 0
    for i = idx, headers.rows - 1 do -- Skip the last, maybe incomplete, row.
        pnt_cnt = pnt_cnt + 1
        local point = {time}
        for j = 1, num_cols do
            point[j+1] = rows[i][j]
            if point[j+1] == "nan" then
                point[j+1] = 0/0
            end
        end
        points[pnt_cnt] = point
        time = time + headers.seconds_per_row
    end

    return {{
        points = points,
        name = data_name,
        columns = col_names,
    }}
end

function process_message()
    local headers, rows = cbuf_utils.parse_cbuf(read_message("Payload"))
    if not headers then
        return -1
    end

    -- Extract message key and look up last time for the message key.
    local msg_key = interp.interpolate_from_msg(msg_key_template)
    local last_time = last_times[msg_key]
    local start_idx = cbuf_utils.get_start_idx(last_time, headers)
    if start_idx < 0 then
        return start_idx
    end

    -- We've got our starting index, use it to generate the json.
    local data_name = interp.interpolate_from_msg(data_name_template)
    local output = generate_data(data_name, headers, rows, start_idx)
    cjson.encode_invalid_numbers("null")
    inject_payload("json", "influx_stats", cjson.encode(output))
    last_times[msg_key] = headers.time
    return 0
end
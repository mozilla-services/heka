-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to JSON for InfluxDB HTTP API. Includes
all standard message fields and iterates through all of the dynamically
specified fields, skipping any bytes fields.

Config:

- series (string, optional, default "series")
    String to use as the `series` key's value in the generated JSON.

*Example Heka Configuration*

.. code-block:: ini

    [influxdb]
    type = "SandboxEncoder"
    filename = "lua_encoders/influxdb.lua"
        [influxdb.config]
        series = "seriesName"

    [InfluxOutput]
    message_matcher = "Type == 'infuxdb'"
    encoder = "influxdb"
    type = "HttpOutput"
    address = "http://influxdbserver.example.com:8086/db/databasename/series"
    username = "influx_username"
    password = "influx_password"

*Example Output*

.. code-block:: json

    [{"points":[[1.409378221e+21,"log","test","systemName",0,"TcpInput",5,"",1,"test"]],"name":"seriesName","columns":["Time","Type","Payload","Hostname","Pid","Logger","Severity","EnvVersion","syslogfacility","programname"]}]

--]=]

require "cjson"

local series  = read_config("series") or "series"

function process_message()
    local ts = read_message("Timestamp")
    ts = ts / (1e6) --Convert to milliseconds

    local columns = {}
    local values = {}

    columns[1] = "time" -- InfluxDB's default
    values[1] = ts

    columns[2] = "Type"
    values[2] = read_message("Type")

    columns[3] = "Payload"
    values[3] = read_message("Payload")

    columns[4] = "Hostname"
    values[4] = read_message("Hostname")

    columns[5] = "Pid"
    values[5] = read_message("Pid")

    columns[6] = "Logger"
    values[6] = read_message("Logger")

    columns[7] = "Severity"
    values[7] = read_message("Severity")

    columns[8] = "EnvVersion"
    values[8] = read_message("EnvVersion")

    local place = 9

    while true do
        local typ, name, value, representation, count = read_next_field()
        if not typ then break end

        if name ~= "Timestamp" and typ ~= 1 then -- exclude bytes
            columns[place] = name
            values[place] = value
            place = place + 1
        end
    end
    local output = {
        {
            name = series,
            columns = columns,
            points =  {values}
        }
    }
    inject_payload("json", "influx_message", cjson.encode(output))
    return 0
end

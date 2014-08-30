-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Converts message payload to JSON for InfluxDB HTTP API.

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

    [{"points":[ [1.409378221e+21,"log","test","systemName",0,"TcpInput",5,"",1,"test"] ],"name":"series","columns":["Time","Type","Payload","Hostname","Pid","Logger","Severity","EnvVersion","syslogfacility","programname"] } ]

--]]

require "cjson"

function process_message()
    local series  = read_config("series") or "series"

    local ts = tonumber(read_message("Timestamp"))
    if not ts then return -1 end
    ts = ts / (1000*1000) --Convert to milliseconds

    local columns = {}
    local values = {}

    columns[#columns+1] = "time" -- InfluxDB's default
    values[#values+1] = ts

    columns[#columns+1] = "Type"
    values[#values+1] = read_message("Type")

    columns[#columns+1] = "Payload"
    values[#values+1] = read_message("Payload")

    columns[#columns+1] = "Hostname"
    values[#values+1] = read_message("Hostname")

    columns[#columns+1] = "Pid"
    values[#values+1] = read_message("Pid")

    columns[#columns+1] = "Logger"
    values[#values+1] = read_message("Logger")

    columns[#columns+1] = "Severity"
    values[#values+1] = read_message("Severity")

    columns[#columns+1] = "EnvVersion"
    values[#values+1] = read_message("EnvVersion")



    while true do
        typ, name, value, representation, count = read_next_field()
        if not typ then break end

        if name ~= "Timestamp" and typ ~= 1 then -- exclude bytes

            columns[#columns+1] = name
            values[#values+1] = value

        end
        count = count + 1
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

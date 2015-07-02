-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to line protocol for InfluxDB HTTP write API
(new in InfluxDB v0.9.0).  Optionally includes all standard message fields
as tags or fields and iterates through all of the dynamic fields to add as
points (series), skipping any fields explicitly omitted using the `skip_fields`
config option.  It can also map any Heka message fields as tags in the
request sent to the InfluxDB write API, using the `tag_fields` config option.
All dynamic fields in the Heka message are converted to separate points
separated by newlines that are submitted to InfluxDB.

.. note::
    This encoder is intended for use with InfluxDB versions 0.9 or greater. If
    you're working with InfluxDB versions prior to 0.9, you'll want to use the
    :ref:`config_schema_influx_encoder` instead.

Config:

- decimal_precision (string, optional, default "6")
    String that is used in the string.format function to define the number
    of digits printed after the decimal in number values.  The string formatting
    of numbers is forced to print with floating points because InfluxDB will
    reject values that change from integers to floats and vice-versa.  By
    forcing all numbers to floats, we ensure that InfluxDB will always
    accept our numerical values, regardless of the initial format.

- exclude_name (boolean, optional, default false)
    Setting this to true will remove the name_prefix_delimiter and field
    name from the metric name in the output of this encoder.  This is useful
    if you have the metric name in a separate field from the value and would
    rather use field interpolation to include the metric name in the
    name_prefix config option to format the metric name. Be careful to not set
    this to true and leave the defaults of name_prefix as that will result in
    a blank name value.

- name_prefix (string, optional, default "")
    String to use as the `name` key's prefix value in the generated line.
    Supports :ref:`message field interpolation<sandbox_msg_interpolate_module>`.
    `%{fieldname}`. Any `fieldname` values of "Type", "Payload", "Hostname",
    "Pid", "Logger", "Severity", or "EnvVersion" will be extracted from the
    the base message schema, any other values will be assumed to refer to a
    dynamic message field. Only the first value of the first instance of a
    dynamic message field can be used for name name interpolation. If the
    dynamic field doesn't exist, the uninterpolated value will be left in the
    name. Note that it is not possible to interpolate either the
    "Timestamp" or the "Uuid" message fields into the name, those
    values will be interpreted as referring to dynamic message fields.

- name_prefix_delimiter (string, optional, default "")
    String to use as the delimiter between the name_prefix and the field
    name.  This defaults to a blank string but can be anything else
    instead (such as "." to use Graphite-like naming).

- skip_fields (string, optional, default "")
    Space delimited set of fields that should *not* be included in the
    InfluxDB records being generated. Any fieldname values of "Type",
    "Payload", "Hostname", "Pid", "Logger", "Severity", or "EnvVersion" will
    be assumed to refer to the corresponding field from the base message
    schema. Any other values will be assumed to refer to a dynamic message
    field. The magic value "**all_base**" can be used to exclude base fields
    from being mapped to the event altogether (useful if you don't want to
    use tags and embed them in the name_prefix instead).

- tag_fields (string, optional, default "**all_base**")
    Take fields defined and adds them as tags of the point(s) sent to
    InfluxDB for the message.  The magic values "**all**" and "**all_base**"
    are used to map all non-numeric fields (including base) to tags and only
    base fields to tags, respectively.  If those magic values aren't used,
    then only those fields defined will map to tags of the points sent to InfluxDB.
    The tag_fields values are independent of the skip_fields values and have
    no affect on each other.  You can skip fields from being sent to InfluxDB
    as points (metrics), but still include them as tags, as long as they are not
    numbers (those are filtered out automatically).

- timestamp_precision (string, optional, default "ms")
    Specify the timestamp precision that you want the event sent with.  The
    default is to use milliseconds by dividing the Heka message timestamp
    by 1e6, but this math can be altered by specifying one of the precision
    values supported by the InfluxDB write API (ms, s, m, h). Other precisions
    supported by InfluxDB of n and u are not yet supported.

- value_field_name (string, optional, default "value")
    This defines the name of the measurement.  We default this to "value"
    to match the examples in the InfluxDB documentation, but can be overrided
    to anything else that you prefer.

*Example Heka Configuration*

.. code-block:: ini

    [LoadAvgPoller]
    type = "FilePollingInput"
    ticker_interval = 5
    file_path = "/proc/loadavg"
    decoder = "LinuxStatsDecoder"

    [LoadAvgDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_loadavg.lua"

    [LinuxStatsDecoder]
    type = "MultiDecoder"
    subs = ["LoadAvgDecoder", "AddStaticFields"]
    cascade_strategy = "all"
    log_sub_errors = false

    [AddStaticFields]
    type = "ScribbleDecoder"

        [AddStaticFields.message_fields]
        Environment = "dev"

    [InfluxdbLineEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/schema_influx_line.lua"

        [InfluxdbLineEncoder.config]
        skip_fields = "**all_base** FilePath NumProcesses Environment TickerInterval"
        tag_fields = "Hostname Environment"
        timestamp_precision= "s"

    [InfluxdbOutput]
    type = "HttpOutput"
    message_matcher = "Type =~ /stats.*/"
    encoder = "InfluxDBWriteEncoder"
    address = "http://influxdbserver.example.com:8086/write?db=mydb&rp=mypolicy&precision=s"
    username = "influx_username"
    password = "influx_password"

*Example Output*

.. code-block:: none

    5MinAvg,Hostname=myhost,Environment=dev value=0.110000 1434932024
    1MinAvg,Hostname=myhost,Environment=dev value=0.110000 1434932024
    15MinAvg,Hostname=myhost,Environment=dev value=0.170000 1434932024

--]=]

local line_protocol = require "line_protocol"

-- Variables required by line_protocol module
line_protocol.timestamp_precision = read_config("timestamp_precision") or "ms"
line_protocol.decimal_precision = read_config("decimal_precision") or "6"
line_protocol.exclude_name = read_config("exclude_name") or false
line_protocol.name_prefix = read_config("name_prefix") or ""
line_protocol.name_prefix_delimiter = read_config("name_prefix_delimiter") or ""
line_protocol.tag_fields = read_config("tag_fields") or "**all_base**"
line_protocol.value_field_name = read_config("value_field_name") or "value"

function process_message()
    local api_message = line_protocol.influxdb_line_msg()

    -- Inject a new message with the payload populated with the newline
    -- delimited data points, and append a newline at the end for the last line
    inject_payload("txt", "influx_line", table.concat(api_message, "\n").."\n")

    return 0
end


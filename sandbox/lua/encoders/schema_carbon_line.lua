-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[=[
Converts full Heka message contents to line protocol for Carbon Plaintext API
Iterates through all of the dynamic fields to add as points (series), skipping
any fields explicitly omitted using the `skip_fields` config option.  All
dynamic fields in the Heka message are converted to separate points separated
by newlines that are submitted to Carbon.

Config:

- name_prefix (string, optional, default nil)
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

- name_prefix_delimiter (string, optional, default ".")
    String to use as the delimiter between the name_prefix and the field
    name.  This defaults to a "." to use Graphite naming convention.

- skip_fields (string, optional, default nil)
    Space delimited set of fields that should *not* be included in the
    Carbon records being generated. Any fieldname values of "Type",
    "Payload", "Hostname", "Pid", "Logger", "Severity", or "EnvVersion" will
    be assumed to refer to the corresponding field from the base message
    schema. Any other values will be assumed to refer to a dynamic message
    field. The magic value "**all_base**" can be used to exclude base fields
    from being mapped to the event altogether.

- source_value_field (string, optional, default nil)
    If the desired behavior of this encoder is to extract one field from the
    Heka message and feed it as a single line to Carbon, then use this option
    to define which field to find the value from.  Make sure to set the
    name_prefix value to use fields from the message with field interpolation
    so the full metric path in Graphite is populated.  When this option is
    present, no other fields besides this one will be sent to Carbon whatsoever.

*Example Heka Configuration*

.. code-block:: ini

    [LinuxStatsDecoder]
    type = "MultiDecoder"
    subs = ["LoadAvgDecoder", "AddStaticFields"]
    cascade_strategy = "all"
    log_sub_errors = false

    [LoadAvgPoller]
    type = "FilePollingInput"
    ticker_interval = 5
    file_path = "/proc/loadavg"
    decoder = "LinuxStatsDecoder"

    [LoadAvgDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/linux_loadavg.lua"

    [AddStaticFields]
    type = "ScribbleDecoder"

        [AddStaticFields.message_fields]
        Environment = "dev"

    [CarbonLineEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/schema_carbon_line.lua"

        [CarbonLineEncoder.config]
        name_prefix = "%{Environment}.%{Hostname}.%{Type}"
        skip_fields = "**all_base** FilePath NumProcesses Environment TickerInterval"

    [CarbonOutput]
    type = "TcpOutput"
    message_matcher = "Type =~ /stats.*/"
    encoder = "CarbonLineEncoder"
    address = "127.0.0.1:2003"

*Example Output*

.. code-block:: none

    dev.myhost.stats.loadavg.1MinAvg 0.12 1434932023
    dev.myhost.stats.loadavg.15MinAvg 0.18 1434932023
    dev.myhost.stats.loadavg.5MinAvg 0.11 1434932023

--]=]

local ts_line_protocol = require "ts_line_protocol"

local decoder_config = {
    carbon_format = true,
    timestamp_precision = "s",
    name_prefix = read_config("name_prefix") or nil,
    name_prefix_delimiter = read_config("name_prefix_delimiter") or ".",
    skip_fields_str = read_config("skip_fields") or nil,
    source_value_field = read_config("source_value_field") or nil
}

local config = ts_line_protocol.set_config(decoder_config)

function process_message()
    local api_message = ts_line_protocol.carbon_line_msg(config)

    -- Inject a new message with the payload populated with the newline
    -- delimited data points, and append a newline at the end for the last line
    inject_payload("txt", "carbon_line", table.concat(api_message, "\n").."\n")

    return 0
end


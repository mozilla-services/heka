-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Prepends ElasticSearch BulkAPI index JSON to a message payload.

Config:

- index (string, optional, default "heka-%{%Y.%m.%d}")
    String to use as the `_index` key's value in the generated JSON. Supports
    field interpolation as described below.
- type_name (string, optional, default "message")
    String to use as the `_type` key's value in the generated JSON. Supports
    field interpolation as described below.
- id (string, optional)
    String to use as the `_id` key's value in the generated JSON. Supports
    field interpolation as described below.
- es_index_from_timestamp (boolean, optional)
    If true, then any time interpolation (often used to generate the
    ElasticSeach index) will use the timestamp from the processed message
    rather than the system time.

Field interpolation:

    Data from the current message can be interpolated into any of the string
    arguments listed above. A `%{}` enclosed field name will be replaced by
    the field value from the current message. Supported default field names
    are "Type", "Hostname", "Pid", "UUID", "Logger", "EnvVersion", and
    "Severity". Any other values will be checked against the defined dynamic
    message fields. If no field matches, then a `C strftime
    <http://man7.org/linux/man-pages/man3/strftime.3.html>`_ (on non-Windows
    platforms) or `C89 strftime <http://msdn.microsoft.com/en-
    us/library/fe06s4ak.aspx>`_ (on Windows) time substitution will be
    attempted.

*Example Heka Configuration*

.. code-block:: ini

    [es_payload]
    type = "SandboxEncoder"
    filename = "lua_encoders/es_payload.lua"
        [es_payload.config]
        es_index_from_timestamp = true
        index = "%{Logger}-%{%Y.%m.%d}"
        type_name = "%{Type}-%{Hostname}"

    [ElasticSearchOutput]
    message_matcher = "Type == 'mytype'"
    encoder = "es_payload"

*Example Output*

{"index":{"_index":"mylogger-2014.06.05","_type":"mytype-host.domain.com"}}
{"json":"data","extracted":"from","message":"payload"}
--]]

require "string"
local elasticsearch = require "elasticsearch"

local ts_from_message = read_config("es_index_from_timestamp")
local index = read_config("index") or "heka-%{%Y.%m.%d}"
local type_name = read_config("type_name") or "message"
local id = read_config("id")

function process_message()
    local ns
    if ts_from_message then
        ns = read_message("Timestamp")
    end
    local idx_json = elasticsearch.bulkapi_index_json(index, type_name, id, ns)
    add_to_payload(idx_json, "\n", read_message("Payload"))
    inject_payload()
    return 0
end

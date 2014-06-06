-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
---
**bulkapi_index_json(index, type_name, id, ns)**

    Returns a simple JSON 'index' structure satisfying the `ElasticSearch
    BulkAPI
    <http://www.elasticsearch.org/guide/en/elasticsearch/reference/current
    /docs-bulk.html>`_

    *Arguments*
        - index (string or nil)
            String to use as the `_index` key's value in the generated JSON,
            or nil to omit the key. Supports field interpolation as described
            below.
        - type_name (string or nil)
            String to use as the `_type` key's value in the generated JSON, or
            nil to omit the key. Supports field interpolation as described
            below.
        - id (string or nil) 
            String to use as the `_id` key' value in the generated JSON, or
            nil to omit the key. Supports field interpolation as described
            below.
        - ns (number or nil)
            Nanosecond timestamp to use for any strftime field interpolation
            into the above fields. Current system time will be used if nil.

    *Field interpolation*

        Data from the current message can be interpolated into any of the
        string arguments listed above. A `%{}` enclosed field name will be
        replaced by the field value from the current message. Supported
        default field names are "Type", "Hostname", "Pid", "UUID", "Logger",
        "EnvVersion", and "Severity". Any other values will be checked against
        the defined dynamic message fields. If no field matches, then a `C
        strftime <http://man7.org/linux/man-pages/man3/strftime.3.html>`_ (on
        non-Windows platforms) or `C89 strftime <http://msdn.microsoft.com/en-
        us/library/fe06s4ak.aspx>`_ (on Windows) time substitution will be
        attempted, using the nanosecond timestamp (if provided) or the system
        clock (if not).

    *Return*
        - JSON string suitable for use as ElasticSearch BulkAPI index
          directive.
--]]

local os = require "os"
local string = require "string"
local cjson = require "cjson"
local read_message = read_message
local tostring = tostring
local type = type

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

local secs
local pattern = "%%{(.-)}"

local interp_fields = {
    Type = "Type",
    Hostname = "Hostname",
    Pid = "Pid",
    UUID = "Uuid",
    Logger = "Logger",
    EnvVersion = "EnvVersion",
    Severity = "Severity"
}

local result_inner = {
    _index = nil,
    _type = nil,
    _id = nil
}

local function interpolate_match(match)
    -- First see if it's a primary message schema field.
    local fname = interp_fields[match]
    if fname then
        return read_message(fname)
    end
    -- Second check for a dynamic field.
    fname = string.format("Fields[%s]", match)
    local fval = read_message(fname)
    if type(fval) == "boolean" then
        return tostring(fval)
    elseif fval then
        return fval
    end
    -- Finally try to use it as a strftime format string.
    fval = os.date(match, secs)
    if fval ~= match then  -- Only return it if a substitution happened.
        return fval
    end
end

--[[ Public Interface --]]

function bulkapi_index_json(index, type_name, id, ns)
    if ns then
        secs = ns / 1e9
    else
        secs = nil
    end
    if index then
        result_inner._index = string.gsub(index, pattern, interpolate_match)
    else
        result_inner._index = nil
    end
    if type_name then
        result_inner._type = string.gsub(type_name, pattern, interpolate_match)
    else
        result_inner._type = nil
    end
    if id then
        result_inner._id = string.gsub(id, pattern, interpolate_match)
    else
        result_inner._id = nil
    end
    return cjson.encode({index = result_inner})
end

return M

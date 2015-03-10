-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
^^^
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

    All of the string arguments listed above support :ref:`message field
    interpolation<sandbox_msg_interpolate_module>`.

    *Return*
        - JSON string suitable for use as ElasticSearch BulkAPI index
          directive.
--]]

local cjson = require "cjson"
local interp = require "msg_interpolate"
local os = require "os"
local string = require "string"
local read_message = read_message
local tostring = tostring
local type = type

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

local result_inner = {
    _index = nil,
    _type = nil,
    _id = nil
}

--[[ Public Interface --]]

function bulkapi_index_json(index, type_name, id, ns)
    if ns then
        secs = ns / 1e9
    else
        secs = nil
    end
    if index then
        result_inner._index = interp.interpolate_from_msg(index, secs)
    else
        result_inner._index = nil
    end
    if type_name then
        result_inner._type = interp.interpolate_from_msg(type_name, secs)
    else
        result_inner._type = nil
    end
    if id then
        result_inner._id = interp.interpolate_from_msg(id, secs)
    else
        result_inner._id = nil
    end
    return cjson.encode({index = result_inner})
end

return M

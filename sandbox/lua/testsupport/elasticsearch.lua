-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "string"
require "os"
local elasticsearch = require "elasticsearch"

local function test_static()
    local res = elasticsearch.bulkapi_index_json("index", "type_name", "id")
    assert(res == '{"index":{"_index":"index","_type":"type_name","_id":"id"}}')
end

local function test_static_missing()
    local res = elasticsearch.bulkapi_index_json("index")
    assert(res == '{"index":{"_index":"index"}}')
end

local function test_basic_interp()
    local res = elasticsearch.bulkapi_index_json("index-%{Logger}", "%{Type}")
    assert(res == '{"index":{"_index":"index-GoSpec","_type":"TEST"}}')
end

local function test_multiple_interp()
    local res = elasticsearch.bulkapi_index_json("%{Logger}-%{Type}-%{Severity}")
    assert(res == '{"index":{"_index":"GoSpec-TEST-6"}}')
end

local function test_field_interp()
    local res = elasticsearch.bulkapi_index_json("%{foo}-%{int}-%{bool}-%{false}")
    assert(res == '{"index":{"_index":"bar-999-true-false"}}')
end

local function test_interp_no_match()
    local res = elasticsearch.bulkapi_index_json("%{foo}-%{Payload}-%{double}-%{missing}")
    assert(res == '{"index":{"_index":"bar-%{Payload}-99.9-%{missing}"}}')
end

local function test_time_interp()
    local res = elasticsearch.bulkapi_index_json("%{Logger}-%{date:%Y-%d-%m}")
    idx = string.format("GoSpec-date:%s", os.date("%Y-%d-%m"))
    assert(res == string.format('{"index":{"_index":"%s"}}', idx))
end

local function test_time_provided()
    local res = elasticsearch.bulkapi_index_json("%{%b %d, %Y}", nil, nil, 512345678900000000)
    assert(res == '{"index":{"_index":"Mar 27, 1986"}}')
end

function process_message()
    test_static()
    test_static_missing()
    test_basic_interp()
    test_multiple_interp()
    test_field_interp()
    test_interp_no_match()
    test_time_interp()
    test_time_provided()
    return 0
end

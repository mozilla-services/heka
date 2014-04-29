-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "circular_buffer"

local annotation = require "annotation"
local anomaly = require "anomaly"

local function test_parse()
    assert(not pcall(anomaly.parse_config, "bogus"))
end

local function test_roc_loss_of_data()
    local cfg = anomaly.parse_config('roc("Output1", 1, 15, 0, 1.5, true, false)')
    local cb = circular_buffer.new(100, 1, 1)
    for x = 100, 150 do
        cb:set(x*1e9, 1, 2)
    end
    cb:set(171*1e9, 1, 0/0)
    local msg, a = anomaly.detect(170*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: roc col: 1 msg: no new data", msg)
    assert(#a == 1, #a)
end

local function test_roc_start_of_data()
    local cfg = anomaly.parse_config('roc("Output1", 1, 15, 0, 1.5, false, true)')
    local cb = circular_buffer.new(100, 1, 1)
    cb:set(170*1e9, 1, 2)
    local val = cb:get(171*1e9, 1)
    local msg, a = anomaly.detect(171*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: roc col: 1 msg: unexpected data", msg)
end

local function test_roc()
    local cfg = anomaly.parse_config('roc("Output1", 1, 15, 0, 1.5, true, false)')
    local cb = circular_buffer.new(100, 1, 1)
    for x = 100, 150 do
        cb:set(x*1e9, 1, x-99)
    end
    for x = 151, 184 do
        local val = x%4 + 9
        cb:set(x*1e9, 1, val)
    end
    for x = 185, 199 do
        cb:set(x*1e9, 1, 23)
    end
    local msg, a = anomaly.detect(200*1e9, "Output1", cb, cfg)
    assert(not msg, msg)
    assert(not a)

    cfg = anomaly.parse_config('roc("Output1", 1, 15, 15, 1.5, true, false)')
    msg, a = anomaly.detect(200*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: roc col: 1 msg: detected anomaly, standard deviation exceeds 1.5", msg)
    assert(#a == 1, #a)
    assert(a[1].x == 200 * 1e3, a[1].x)
    assert(a[1].col, a[1].col)
    assert(a[1].shortText == "A", a[1].shortText)
    assert(a[1].text == "detected anomaly, standard deviation exceeds 1.5", a[1].text)
end

local function test_roc_out_of_range()
    local cfg = anomaly.parse_config('roc("Output1", 1, 15, 0, 1.5, true, false)')
    local cb = circular_buffer.new(45, 1, 1)
    assert(not pcall(anomaly.detect, 44*1e9, "Output1", cb, cfg))

    cfg = anomaly.parse_config('roc("Output1", 1, 10, 5, 1.5, true, false)')
    assert(not pcall(anomaly.detect, 44*1e9, "Output1", cb, cfg))
end

local function test_mww_decreasing()
    local cfg = anomaly.parse_config('mww("Output1", 1, 20, 10, 0.0001, decreasing)')
    local cb = circular_buffer.new(220, 1, 1)
    local i = 1000
    for x = 300, 520 do
        cb:set(x*1e9, 1, i)
        i = i - 1
    end
    local msg, a = anomaly.detect(521*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: mww col: 1 msg: detected anomaly, decreasing values", msg)
    assert(#a == 1, #a)
end

local function test_mww_increasing()
    local cfg = anomaly.parse_config('mww("Output1", 1, 20, 10, 0.0001, increasing)')
    local cb = circular_buffer.new(220, 1, 1)
    local i = 1000
    for x = 300, 520 do
        cb:set(x*1e9, 1, i)
        i = i + 1
    end
    local msg, a = anomaly.detect(520*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: mww col: 1 msg: detected anomaly, increasing values", msg)
    assert(#a == 1, #a)
end

local function test_mww_any()
    local cfg = anomaly.parse_config('mww("Output1", 1, 20, 10, 0.0001, any)')
    local cb = circular_buffer.new(220, 1, 1)
    local i = 1000
    for x = 300, 520 do
        cb:set(x*1e9, 1, i)
        i = i - 1
    end
    local msg, a = anomaly.detect(521*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: mww col: 1 msg: detected anomaly", msg)
    assert(#a == 1, #a)
end

local function test_mww_out_of_range()
    local cfg = anomaly.parse_config('mww("Output1", 1, 20, 10, 0.0001, any)')
    local cb = circular_buffer.new(200, 1, 1)
    assert(not pcall(anomaly.detect, 200*1e9, "Output1", cb, cfg))
end

local function test_all_nan()
    local cfg = anomaly.parse_config('mww("Output1", 1, 20, 10, 0.0001, decreasing)')
    local cb = circular_buffer.new(220, 1, 1)
    local msg, a = anomaly.detect(219*1e9, "Output1", cb, cfg)
    assert(not msg, msg)
    assert(not a)

    cfg = anomaly.parse_config('roc("Output1", 1, 15, 0, 1.5, true, false)')
    msg, a = anomaly.detect(219*1e9, "Output1", cb, cfg)
    assert(not msg, msg)
    assert(not a)
end

local function test_mww_nonparametric_increasing()
    local cfg = anomaly.parse_config('mww_nonparametric("Output1", 1, 20, 10, 0.6)')
    local cb = circular_buffer.new(220, 1, 1)
    local i = 1000
    for x = 300, 520 do
        cb:set(x*1e9, 1, i)
        i = i + 1
    end
    local msg, a = anomaly.detect(520*1e9, "Output1", cb, cfg)
    assert(msg == "Output1 - algorithm: mww_nonparametric col: 1 msg: detected anomaly, pstat: 0.944444", msg)
    assert(#a == 1, #a)
end

function process_message ()
    test_parse()
    test_roc_loss_of_data()
    test_roc_start_of_data()
    test_roc()
    test_roc_out_of_range()
    test_mww_decreasing()
    test_mww_increasing()
    test_mww_any()
    test_mww_out_of_range()
    test_all_nan()
    test_mww_nonparametric_increasing()
    return 0
end


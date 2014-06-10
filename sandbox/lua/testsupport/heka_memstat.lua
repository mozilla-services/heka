-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

require "circular_buffer"

local rows           = read_config("rows") or 1440
local sec_per_row    = read_config("sec_per_row") or 60

cbuf = circular_buffer.new(rows, 6, sec_per_row)
local HEAP_SYS      = cbuf:set_header(1, "HeapSys"     , "B"    , "max")
local HEAP_ALLOC    = cbuf:set_header(2, "HeapAlloc"   , "B"    , "max")
local HEAP_IDLE     = cbuf:set_header(3, "HeapIdle"    , "B"    , "max")
local HEAP_INUSE    = cbuf:set_header(4, "HeapInuse"   , "B"    , "max")
local HEAP_RELEASED = cbuf:set_header(5, "HeapReleased", "B"    , "max")
local HEAP_OBJECTS  = cbuf:set_header(6, "HeapObjects" , "count", "max")

function process_message ()
    local ts = read_message("Timestamp")

    cbuf:set(ts, HEAP_SYS       , read_message("Fields[HeapSys]"))
    cbuf:set(ts, HEAP_ALLOC     , read_message("Fields[HeapAlloc]"))
    cbuf:set(ts, HEAP_IDLE      , read_message("Fields[HeapIdle]"))
    cbuf:set(ts, HEAP_INUSE     , read_message("Fields[HeapInuse]"))
    cbuf:set(ts, HEAP_RELEASED  , read_message("Fields[HeapReleased]"))
    cbuf:set(ts, HEAP_OBJECTS   , read_message("Fields[HeapObjects]"))
    return 0
end

function timer_event(ns)
    inject_payload("cbuf", "MemStats", cbuf)
end

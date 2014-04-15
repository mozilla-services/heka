-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
---
**add(name, ns, col, stext, text)**
    Create an annotation in the global *_ANNOTATIONS* table.

    *Arguments*
        - name (string) circular buffer payload name.
        - ns (int64) current time in nanoseconds since the UNIX epoch.
        - col (uint) circular buffer column to annotate.
        - stext (string) short text to display on the graph.
        - text (string) long text to display in the rollover.

    *Return*
        - none

**create(ns, col, stext, text)**
    Helper function to create an annotation table but not add it to the
    global list of annotations.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch.
        - col (uint) circular buffer column to annotate.
        - stext (string) short text to display on the graph.
        - text (string) long text to display in the rollover.

    *Return*
        - annotation table

**concat(name, annotations)**
    Concatenates an array of annotation tables to the specified key in the
    global _ANNOTATIONS table.

    *Arguments*
        - name (string) circular buffer payload name.
        - annotations (array) annotation tables.

    *Return*
        - none

**prune(name, ns)**

    *Arguments*
        - name (string) circular buffer payload name.
        - ns (int64) current time in nanoseconds since the UNIX epoch.

    *Return*
        - The pruned list of annotations.

**remove(name)**
    Entirely remove the payload name from the global *_ANNOTATIONS* table.

    *Arguments*
        - name (string) circular buffer payload name.

    *Return*
        - none

**set_prune(name, ns_duration)**

    *Arguments*
        - name (string) circular buffer payload name.
        - ns_duration (int64) time in nanoseconds the annotation should remain in the list.

    *Return*
        - none
--]]

-- Global Exports

_ANNOTATIONS = {} -- throw the annotations into global space so they are
                  -- preserved. Not really liking this but from a usability
                  -- and preservation perspective it makes things more seamless.

-- Imports
local math  = require "math"
local table = require "table"
local ipairs = ipairs
local _ANNOTATIONS = _ANNOTATIONS

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module

local prune_duration = 60 * 60 * 24 * 1e9 -- default it to a day

local function create_key(name)
    local a = _ANNOTATIONS[name]
    if not a then
        a = {_prune_duration = prune_duration}
        _ANNOTATIONS[name] = a
    end
    return a
end


function create(ns, col, stext, text)
    return {x = math.floor(ns/1e6), col = col, shortText = stext, text = text}
end


function add(name, ns, col, stext, text)
    local a = create_key(name)
    table.insert(a, create(ns, col, stext, text))
end


function remove(name)
    _ANNOTATIONS[name] = nil
end


function concat(name, annotations)
    local a = create_key(name)
    for i, v in ipairs(annotations) do
        table.insert(a, v)
    end
end


function prune(name, ns)
    local a = _ANNOTATIONS[name]
    if not a then
        return
    end

    local len = #a
    local deletion = false
    for i = 1, len do
        if a[i].x * 1e6 + a._prune_duration <= ns then
           a[i] = nil
           deletion = true
        end
    end

    if deletion then
        local j = 1
        for i = 1, len do
            if a[i] then
                if j ~= i then
                    a[j] = a[i]
                    a[i] = nil
                end
                j = j + 1
            end
        end
    end

    return a
end


function set_prune(name, ns_duration)
    local a = create_key(name)
    a._prune_duration = ns_duration
end

return M

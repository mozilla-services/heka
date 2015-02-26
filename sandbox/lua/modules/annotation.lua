-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
^^^
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
        - The json encoded list of annotations.

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
require "math"
require "cjson"
require "table"

local M = {}
-- We cannot remove external access because we need to use the global
-- _ANNOTATIONS which will not be the same table as we import after a data
-- restoration.

local prune_duration = 60 * 60 * 24 * 1e9 -- default it to a day

-- todo temporary schema migration, remove before 0.6 release
local function fix_up_schema(name, a)
    if a and not a.annotations then
        t = {}
        t.prune_duration = a._prune_duration
        a._prune_duration = nil
        t.annotations = a
        _ANNOTATIONS[name] = t
        return t
    end
    return a
end
-- end todo

local function create_key(name)
    local a = _ANNOTATIONS[name]
    a = fix_up_schema(name, a) -- todo remove
    if not a then
        a = {annotations = {}, prune_duration = prune_duration}
        _ANNOTATIONS[name] = a
    end
    return a
end


function M.create(ns, col, stext, text)
    return {x = math.floor(ns/1e6), col = col, shortText = stext, text = text}
end


function M.add(name, ns, col, stext, text)
    local a = create_key(name)
    table.insert(a.annotations, M.create(ns, col, stext, text))
end


function M.remove(name)
    _ANNOTATIONS[name] = nil
end


function M.concat(name, annotations)
    local a = create_key(name)
    for i, v in ipairs(annotations) do
        table.insert(a.annotations, v)
    end
end


function M.prune(name, ns)
    local a = _ANNOTATIONS[name]
    if not a then
        return
    end
    a = fix_up_schema(name, a) -- todo remove

    local len = #a.annotations
    local deletion = false
    for i = 1, len do
        if a.annotations[i].x * 1e6 + a.prune_duration <= ns then
           a.annotations[i] = nil
           deletion = true
        end
    end

    if deletion then
        local j = 1
        for i = 1, len do
            if a.annotations[i] then
                if j ~= i then
                    a.annotations[j] = a.annotations[i]
                    a.annotations[i] = nil
                end
                j = j + 1
            end
        end
    end

    return cjson.encode({annotations = a.annotations}) .. "\n"
end


function M.set_prune(name, ns_duration)
    local a = create_key(name)
    a.prune_duration = ns_duration
end

return M

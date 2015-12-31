-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[

API
---
**interpolate_from_msg(value, secs)**

    Interpolates values from the currently processed message into the provided
    string value. A `%{}` enclosed field name will be replaced by the field
    value from the current message. Supported default field names are "Type",
    "Hostname", "Pid", "UUID", "Logger", "EnvVersion", and "Severity". Any
    other values will be checked against the defined dynamic message fields.
    If no field matches, then a `C strftime <http://man7.org/linux/man-
    pages/man3/strftime.3.html>`_ (on non-Windows platforms) or `C89 strftime
    <http://msdn.microsoft.com/en- us/library/fe06s4ak.aspx>`_ (on Windows)
    time substitution will be attempted. The time used for time substitution
    will be the seconds-from-epoch timestamp passed in as the `secs` argument,
    if provided. If `secs` is nil, local system time is used. Note that the
    message timestamp is *not* automatically used; if you want to use the
    message timestamp for time substitutions, then you need to manually
    extract it and convert it from nanoseconds to seconds (i.e. divide by
    1e9).

    *Arguments*
        - value (string)
            String into which message values should be interpolated.
        - secs (number or nil)
            Timestamp (in seconds since epoch) to use for time substitutions.
            If nil, system time will be used.

    *Return*
        - Original string value with any interpolated message values.
--]]

local os = require "os"
local string = require "string"
local read_message = read_message
local tostring = tostring
local type = type

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module.

local pattern = "%%{(.-)}"
local _secs

local interp_fields = {
    Type = "Type",
    Hostname = "Hostname",
    Pid = "Pid",
    UUID = "Uuid",
    Logger = "Logger",
    EnvVersion = "EnvVersion",
    Severity = "Severity"
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
    fval = os.date(match, _secs)
    if fval ~= match then  -- Only return it if a substitution happened.
        return fval
    end
end

--[[ Public Interface --]]

function interpolate_from_msg(value, secs)
    _secs = secs
    return string.gsub(value, pattern, interpolate_match)
end

return M

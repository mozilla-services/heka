-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

local s = read_config("string")
if s ~= nil then error("string") end

function process_message ()
    return 0
end

function timer_event()
end


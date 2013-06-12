-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

users = {}
users_size = 0
day = math.floor(os.time() / (60 * 60 * 24))

local WEIGHT = 1

function process_message ()
    local user = read_message("Fields[User]")
    if user == nil then return 0 end

    local u = users[user]
    if u == nil  then
        if users_size == 10000 then
            for k,v in pairs(users) do
                v[WEIGHT] = v[WEIGHT] - 1
                if  v[WEIGHT] == 0 then
                    users[k] = nil
                    users_size = users_size - 1
                end
            end
        else
            u = {0}
            users[user] = u
            users_size = users_size + 1
        end
    end
    if u ~= nil then
        u[WEIGHT] = u[WEIGHT] + 1
    end
    return 0
end

function timer_event(ns)
    output("User\tWeight\n")
    for k, v in pairs(users) do
        if v[WEIGHT] > 100 then
            output(string.format("%s\t%d\n", k, v[WEIGHT]))
        end
    end
    inject_message("tsv", "Weighting by User")

    -- reset the frequent users once a day
    local current_day = math.floor(ns / 1e9 / (60 * 60 * 24))
    if current_day ~= day then
        day = current_day
        users = {}
        users_size = 0
    end
end


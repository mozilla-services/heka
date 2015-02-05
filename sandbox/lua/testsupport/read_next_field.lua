-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

function process_message()
    local type, name, value, representation, count = read_next_field()
    if not(type == 0 and name == "foo" and value == "bar" and representation == "" and count == 1) then
        return 1
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 1 and name == "bytes" and value == "data" and representation == ""  and count == 1) then
        return 2
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 2 and name == "int" and value == 999 and representation == "" and count == 2) then
        return 3
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 3 and name == "double" and value == 99.9 and representation == "" and count == 1) then
        return 4
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 4 and name == "bool" and value == true and representation == "" and count == 1) then
        return 5
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 0 and name == "foo" and value == "alternate" and representation == "" and count == 1) then
        return 6
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 4 and name == "false" and value == false and representation == "" and count == 1) then
        return 7
    end
    type, name, value, representation, count = read_next_field()
    if not(type == 1 and name == "empty_bytes" and value == nil and representation == "" and count == 1) then
        return 8
    end
    type, name, value, representation, count = read_next_field()
    if not(type == nil and name == nil and value == nil and representation == nil and count == nil) then
        return 9
    end

    return 0
end


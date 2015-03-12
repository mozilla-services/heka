-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.


function process_message ()
	msg = read_message("Payload")
	if msg == "string field scribble" then
		write_message("Fields[scribble]", "foo")
	end
	if msg == "num field scribble" then
		write_message("Fields[scribble]", 1)
	end
	if msg == "bool field scribble" then
		write_message("Fields[scribble]", true)
	end
	if msg == "set type and payload" then
		write_message("Type", "my_type")
		write_message("Payload", "my_payload")
	end
	if msg == "set field value with representation" then
		write_message("Fields[rep]", "foo", "representation")
	end
	if msg == "set multiple field string values" then
		write_message("Fields[multi]", "first", "", 0)
		write_message("Fields[multi]", "second", "", 1)
	end
	if msg == "set field string array value" then
		write_message("Fields[array]", "first", "", 0, 0)
		write_message("Fields[array]", "second", "", 0, 1)
	end
	if msg == "delete field scribble" then
		write_message("Fields[scribble]", nil)
	end
	if msg == "delete second field of multi" then
		write_message("Fields[multi]", nil, "", 1)
	end
	if msg == "delete second value of array" then
		write_message("Fields[array]", "first", "", 0, 0)
		write_message("Fields[array]", "second", "", 0, 1)
		write_message("Fields[array]", "third", "", 0, 2)
		write_message("Fields[array]", nil, "", 0, 1)
	end
	return 0
end

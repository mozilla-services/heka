-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

function process_message ()
	write_message("Type", "MyType")
	write_message("Logger", "MyLogger")
	write_message("Timestamp", "2013-12-02T07:21:54.904958136Z")
	write_message("Payload", "MyPayload")
	write_message("EnvVersion", "000")
	write_message("Hostname", "MyHostname")
	write_message("Severity", 4)
	write_message("Pid", "12345")
	write_message("Fields[String]", "foo")
	write_message("Fields[Float]", 1.2345)
	write_message("Fields[Int]", 123, "count", 0, 0)
	write_message("Fields[Int]", 456, "count", 0, 1)
	write_message("Fields[Bool]", true)
	write_message("Fields[Bool]", false, "", 1, 0)
	write_message("Fields[Bool]", false, "", 1, 0)
	write_message("Fields[]", "bad idea")
	write_message("Uuid", "550d19b9-58c7-49d8-b0dd-b48cd1c5b305")
	-- Added and removed, tests verify deletion.
	write_message("Fields[delete]", "foo")
	write_message("Fields[delete]", nil)
	-- Deleting a non-existent field doesn't cause an error.
	write_message("Fields[nonexistent]", nil)
	return 0
end

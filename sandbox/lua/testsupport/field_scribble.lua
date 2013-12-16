-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.


function process_message ()
	write_message("Type", "MyType")
	write_message("Timestamp", 1385968914904958136)
	write_message("Payload", "MyPayload")
	write_message("Severity", 4)
	write_message("Fields[String]", "foo")
	write_message("Fields[Float]", 1.3091)
	write_message("Fields[Int]", 123, "count", 0, 0, 1)
	write_message("Fields[Bool]", true)
	return 0
end
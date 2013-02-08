/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2012
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/
package pipeline

import (
	"fmt"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func FilterSpecificationSpec(c gospec.Context) {
	pack := getTestPipelinePack()
	pack.Message = getTestMessage()
	uuidStr := pack.Message.GetUuidString()

	c.Specify("A FilterSpecification", func() {
		malformed := []string{"",
			"bogus",
			"Type = \"test\"",                                                 // invalid operator
			"Pid == \"test=\"",                                                // Pid is not a string
			"Type == \"test\" && (Severity==7 || Payload == \"Test Payload\"", // missing paren
			"Invalid == \"bogus\"",                                            // unknown variable name
		}

		negative := []string{"FALSE",
			"Type == \"test\"&&(Severity==7||Payload==\"Test Payload\")",
			"EnvVersion == \"0.9\"",
			"EnvVersion != \"0.8\"",
			"EnvVersion > \"0.9\"",
			"EnvVersion >= \"0.9\"",
			"EnvVersion < \"0.8\"",
			"EnvVersion <= \"0.7\"",
			"Severity == 5",
			"Severity != 6",
			"Severity < 6",
			"Severity <= 5",
			"Severity > 6",
			"Severity >= 7",
		}

		positive := []string{"TRUE",
			"(Severity == 7 || Payload == \"Test Payload\") && Type == \"TEST\"",
			"EnvVersion == \"0.8\"",
			"EnvVersion != \"0.9\"",
			"EnvVersion > \"0.7\"",
			"EnvVersion >= \"0.8\"",
			"EnvVersion < \"0.9\"",
			"EnvVersion <= \"0.8\"",
			"Hostname != \"\"",
			"Logger == \"GoSpec\"",
			"Pid != 0",
			"Severity != 5",
			"Severity < 7",
			"Severity <= 6",
			"Severity == 6",
			"Severity > 5",
			"Severity >= 6",
			"Timestamp > 0",
			"Type != \"test\"",
			"Type == \"TEST\" && Severity == 6",
			"Type == \"test\" && Severity == 7 || Payload == \"Test Payload\"",
			"Type == \"TEST\"",
			"Type == \"foo\" || Type == \"bar\" || Type == \"TEST\"",
			fmt.Sprintf("Uuid == \"%s\"", uuidStr),
		}

		c.Specify("malformed filter tests", func() {
			for _, v := range malformed {
				_, err := CreateFilterSpecification(v)
				c.Expect(err, gs.Not(gs.IsNil))
			}
		})

		c.Specify("negative filter tests", func() {
			for _, v := range negative {
				fs, err := CreateFilterSpecification(v)
				c.Expect(err, gs.IsNil)
				fs.FilterMsg(pack)
				c.Expect(pack.Blocked, gs.IsTrue)
			}
		})

		c.Specify("positive filter tests", func() {
			for _, v := range positive {
				fmt.Println("filter", v)
				fs, err := CreateFilterSpecification(v)
				c.Expect(err, gs.IsNil)
				fs.FilterMsg(pack)
				c.Expect(pack.Blocked, gs.IsFalse)
			}
		})

	})
}

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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"fmt"
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func JsonPathSpec(c gs.Context) {
	c.Specify("JsonPath can read data", func() {
		var s = `{
	"foo": {
		"bar": [
			{
				"baz": "こんにちわ世界",
				"noo": "aaa"
			},
			{
				"maz": "123",
				"moo": 256
			}
		],
		"boo": {
			"bag": true,
			"bug": false
		}
	}
}
`
		var err error
		var json_path *JsonPath
		var result interface{}

		json_path = new(JsonPath)
		err = json_path.SetJsonText(s)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		result, err = json_path.Find("/foo/bar[0]/baz")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "こんにちわ世界")

		result, err = json_path.Find("/foo/bar[0]/noo")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "aaa")

		result, err = json_path.Find("/foo/bar[1]/maz")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "123")

		result, err = json_path.Find("/foo/bar[1]/moo")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "256.000000000")

		result, err = json_path.Find("/foo/boo/bag")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "true")

		result, err = json_path.Find("/foo/boo/bug")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "false")

		result, err = json_path.Find("/foo/bar[99]/baz")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("/badpath")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("badpath")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("/foo/bar/3428")
		c.Expect(err, gs.Not(gs.IsNil))

	})
}

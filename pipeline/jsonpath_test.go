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
				"moo": 256,
				"muux": 2.10
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
		c.Expect(err, gs.IsNil)

		result, err = json_path.Find("$.foo.bar[0].baz")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "こんにちわ世界")

		result, err = json_path.Find("$.foo.bar[0].noo")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "aaa")

		result, err = json_path.Find("$.foo.bar[1].maz")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "123")

		result, err = json_path.Find("$.foo.bar[1].moo")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "256")

		result, err = json_path.Find("$.foo.bar[1].muux")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "2.10")

		result, err = json_path.Find("$.foo.boo.bag")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "true")

		result, err = json_path.Find("$.foo.boo.bug")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "false")

		result, err = json_path.Find("$.foo.bar[99].baz")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("$.badpath")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("badpath")
		c.Expect(err, gs.Not(gs.IsNil))

		result, err = json_path.Find("$.foo.bar.3428")
		c.Expect(err, gs.Not(gs.IsNil))

		expected_data := `[{"baz":"こんにちわ世界","noo":"aaa"},{"maz":"123","moo":256,"muux":2.10}]`
		result_data, err := json_path.Find("$.foo.bar")
		c.Expect(err, gs.IsNil)
		c.Expect(result_data, gs.Equals, expected_data)

	})

	c.Specify("JsonPath doesn't crash on nil data", func() {
		var err error
		var json_path *JsonPath

		json_path = new(JsonPath)
		err = json_path.SetJsonText("")
		c.Expect(err, gs.Not(gs.IsNil))

		// Searches should return an error
		result, err := json_path.Find("$.foo.bar.3428")
		c.Expect(err, gs.Not(gs.IsNil))
		c.Expect(err.Error(), gs.Equals, "JSON data is nil")
		c.Expect(result, gs.Equals, "")
	})

	c.Specify("JsonPath handles arrays at top level", func() {
		var err error
		var json_path *JsonPath

		json_path = new(JsonPath)
		err = json_path.SetJsonText(`["foo"]`)
		c.Expect(err, gs.IsNil)

		// Searches should return an error
		result, err := json_path.Find("$.[0]")
		c.Expect(err, gs.IsNil)
		c.Expect(result, gs.Equals, "foo")
	})

	c.Specify("JsonPath handles invalid doubly encoded JSON gracefully", func() {
        s := `{
            "request":{
                "parameters":{
                    "invites":"[{\"inviteUserId\":\"123\",\"email\":\"john@doe.com\",\"phone\":\"123\",\"name\":\"John
                    Doe\"}]",
                    "feature":"0"
                }
            }
        }`

		var err error
		var json_path *JsonPath

		json_path = new(JsonPath)
		err = json_path.SetJsonText(s)
		c.Expect(err, gs.Not(gs.IsNil))

		// Searches should return an error
		result, err := json_path.Find("$.[0]")
		c.Expect(err, gs.Not(gs.IsNil))
		c.Expect(err.Error(), gs.Equals, "JSON data is nil")
		c.Expect(result, gs.Equals, "")
	})

}

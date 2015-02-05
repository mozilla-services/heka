/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2015
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Kun Liu (git@lk.vc)
#
# ***** END LICENSE BLOCK *****/

package smtp

import (
	gs "github.com/rafrombrc/gospec/src/gospec"
)

func EncoderSpec(c gs.Context) {

	c.Specify("Base64 Encoding", func() {
		c.Specify("limit output length", func() {
			str, num := encodeBase64LimitChars("Hello", 7)
			c.Expect(num, gs.Equals, 3)
			c.Expect(str, gs.Equals, "SGVs")
		})
		c.Specify("source within limit", func() {
			str, num := encodeBase64LimitChars("Hello", 80)
			c.Expect(num, gs.Equals, 5)
			c.Expect(str, gs.Equals, "SGVsbG8=")
		})
		c.Specify("utf-8 skip half rune", func() {
			str, num := encodeBase64LimitChars("o测试", 8)
			c.Expect(num, gs.Equals, 4)
			c.Expect(str, gs.Equals, "b+a1iw==")
		})
	})

	c.Specify("Quoted-printable Encoding", func() {
		c.Specify("limit output length", func() {
			str, num := encodeQuoPriLimitChars("Hello world", 7)
			c.Expect(num, gs.Equals, 7)
			c.Expect(str, gs.Equals, "Hello_w")
		})
		c.Specify("source within limit", func() {
			str, num := encodeQuoPriLimitChars("Hello\r\n=?=_.", 80)
			c.Expect(num, gs.Equals, 12)
			c.Expect(str, gs.Equals, "Hello=0A=3D=3F=3D=5F=2E.")
		})
		c.Specify("utf-8 skip half rune", func() {
			str, num := encodeQuoPriLimitChars("o测试", 12)
			c.Expect(num, gs.Equals, 4)
			c.Expect(str, gs.Equals, "o=E8=AF=95")
		})
	})

	c.Specify("Subject Encoding", func() {
		c.Specify("subject 1", func() {
			str := encodeSubject("The ï, ö, ë, ä, and é work, \n" +
				"but when adding the ü it doesn't.")
			c.Expect(str, gs.Equals, "Subject: =?utf-8?B?VGhlIMOvLCDD"+
				"tiwgw6ssIMOkLCBhbmQgw6kgd29yaywgCmJ1dCB3aGVu?=\r\n"+
				" =?utf-8?Q?_adding_the_=20=69_it_doesn't.?=")
		})
		c.Specify("subject 2", func() {
			str := encodeSubject("Hello world!")
			c.Expect(str, gs.Equals, "Subject: Hello world!")
		})
		c.Specify("subject 3", func() {
			str := encodeSubject("从前有座山，山里有座庙")
			c.Expect(str, gs.Equals, "Subject: =?utf-8?B?"+
				"5LuO5YmN5pyJ5bqn5bGx77yM5bGx6YeM5pyJ5bqn5bqZ?=")
		})
	})

}

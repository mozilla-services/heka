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

func XPathSpec(c gs.Context) {
	xml_data := "<SOAP-ENV:Envelope xmlns:SOAP-ENV=\"http://schemas.xmlsoap.org/soap/envelope/\"><SOAP-ENV:Body><m:setPresence xmlns:m=\"http://schemas.microsoft.com/winrtc/2002/11/sip\"><m:presentity m:uri=\"test\"><m:availability m:aggregate=\"300\" m:description=\"online\"/><m:activity m:aggregate=\"400\" m:description=\"Active\"/><textnode>blah</textnode><deviceName xmlns=\"http://schemas.microsoft.com/2002/09/sip/client/presence\" name=\"WIN-0DDABKC1UI8\"/></m:presentity></m:setPresence></SOAP-ENV:Body></SOAP-ENV:Envelope>"

	c.Specify("XPath works on text nodes", func() {
		doc, err := NewXMLDocument(xml_data)
		c.Assume(err, gs.IsNil)
		path := "//*[local-name()='textnode']"
		nodes, err := doc.Find(path)
		c.Assume(err, gs.IsNil)
		c.Expect(len(nodes), gs.Equals, 1)
		c.Expect(nodes[0], gs.Equals, "blah")
		doc.Free()
	})

	c.Specify("XPath works on attribute nodes", func() {
		doc, err := NewXMLDocument(xml_data)
		c.Assume(err, gs.IsNil)
		path := "//*[local-name()='deviceName']/@name"
		nodes, err := doc.Find(path)
		c.Assume(err, gs.IsNil)
		c.Expect(len(nodes), gs.Equals, 1)
		c.Expect(nodes[0], gs.Equals, "WIN-0DDABKC1UI8")
		doc.Free()
	})
}

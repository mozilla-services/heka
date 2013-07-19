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
	"github.com/crankycoder/gokogiri/xml"
)

type XMLDocument struct {
	raw_xml string
	doc     *xml.XmlDocument
}

func NewXMLDocument(raw_xml string) (x *XMLDocument, err error) {
	x = &XMLDocument{raw_xml: raw_xml}

	doc, err := xml.Parse([]byte(raw_xml), xml.DefaultEncodingBytes,
		nil, xml.DefaultParseOption, xml.DefaultEncodingBytes)

	if err != nil {
		return
	}
	x.doc = doc
	return
}

func (x *XMLDocument) Free() {
	x.doc.Free()
}

func (x *XMLDocument) Find(xpath string) (result []string, err error) {
	nodes, err := x.doc.Search(xpath)
	if err != nil {
		return
	}
	result = make([]string, len(nodes))
	for i, node := range nodes {
		result[i] = node.Content()
	}
	return
}

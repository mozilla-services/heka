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
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package testsupport

import (
	"github.com/rafrombrc/gospec/src/gospec"
	"strings"
)

func StringContains(actual interface{}, criteria interface{}) (match bool,
	pos gospec.Message, neg gospec.Message, err error) {
	toTest := actual.(string)
	critTest := criteria.(string)
	match = strings.Contains(toTest, critTest)
	pos = gospec.Messagef(toTest, "contains "+critTest)
	neg = gospec.Messagef(toTest, "does not contain "+critTest)
	return
}

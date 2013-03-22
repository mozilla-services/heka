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

/*

Several support structures for use with gospec to ease test comparisons.

*/
package testsupport

import (
	"github.com/rafrombrc/gospec/src/gospec"
	"log"
	"strings"
	"time"
)

var (
	PostTimeout = time.Duration(10 * time.Millisecond)
)

type SimpleT struct{}

func (*SimpleT) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (*SimpleT) Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func StringContains(actual interface{}, criteria interface{}) (match bool,
	pos gospec.Message, neg gospec.Message, err error) {
	toTest := actual.(string)
	critTest := criteria.(string)
	match = strings.Contains(toTest, critTest)
	pos = gospec.Messagef(toTest, "contains "+critTest)
	neg = gospec.Messagef(toTest, "does not contain "+critTest)
	return
}

func StringStartsWith(actual interface{}, criteria interface{}) (match bool,
	pos gospec.Message, neg gospec.Message, err error) {
	actStr := actual.(string)
	critStr := criteria.(string)
	match = actStr[:len(critStr)] == critStr
	pos = gospec.Messagef(actStr, "starts with %s", critStr)
	neg = gospec.Messagef(actStr, "does not start with %s", critStr)
	return
}

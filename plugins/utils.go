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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CheckWritePermission(fp string) (err error) {
	var file *os.File
	filename := filepath.Join(fp, ".hekad.perm_check")
	if file, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644); err == nil {
		errMsgs := make([]string, 0, 3)
		var e error
		if _, e = file.WriteString("ok"); e != nil {
			errMsgs = append(errMsgs, "can't write to test file")
		}
		if e = file.Close(); e != nil {
			errMsgs = append(errMsgs, "can't close test file")
		}
		if e = os.Remove(filename); e != nil {
			errMsgs = append(errMsgs, "can't remove test file")
		}
		if len(errMsgs) > 0 {
			err = fmt.Errorf("errors: %s", strings.Join(errMsgs, ", "))
		}
	}
	return
}

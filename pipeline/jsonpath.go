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
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type JsonPath struct {
	json_data interface{}
	json_text string
}

func NewJsonPath(json_text string) (result *JsonPath, err error) {
	result = &JsonPath{json_text: json_text}
	err = json.Unmarshal([]byte(json_text), &result.json_data)
	if err != nil {
		return
	}
	return result, nil
}

func (j *JsonPath) find(jp string) (result string, err error) {
	if jp == "" {
		return "", errors.New("invalid path")
	}

	// Need to grab a pointer to the top of the data structure
	v := j.json_data

	var re = regexp.MustCompile("^([^0-9\\s\\[][^\\s\\[]*)?(\\[[0-9]+\\])?$")

	for _, token := range strings.Split(jp, "/") {
		sl := re.FindAllStringSubmatch(token, -1)
		if len(sl) == 0 {
			return "", errors.New("invalid path")
		}
		ss := sl[0]
		if ss[1] != "" {
			v = v.(map[string]interface{})[ss[1]]
		}
		if ss[2] != "" {
			i, err := strconv.Atoi(ss[2][1 : len(ss[2])-1])
			if err != nil {
				return "", errors.New("invalid path")
			}
			v = v.([]interface{})[i]
		}
	}

	result = v.(string)
	return result, nil
}

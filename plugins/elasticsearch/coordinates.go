/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2013-2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Tanguy Leroux (tlrx.dev@gmail.com)
#   Rob Miller (rmiller@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package elasticsearch

import (
	"bytes"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	"strconv"
	"strings"
	"time"
)

// ElasticSearchCoordinates stores the coordinates (_index, _type, _id) of an
// ElasticSearch document.
type ElasticSearchCoordinates struct {
	Index                string
	Type                 string
	Id                   string
	Timestamp            *int64
	TimestampFormat      string
	ESIndexFromTimestamp bool
}

// Renders the coordinates of the ElasticSearch document as JSON.
func (e *ElasticSearchCoordinates) PopulateBuffer(m *message.Message, buf *bytes.Buffer) {
	buf.WriteString(`{"index":{"_index":`)

	var (
		err         error
		interpIndex string
		interpType  string
		interpId    string
	)

	interpIndex, err = interpolateFlag(e, m, e.Index)

	buf.WriteString(strconv.Quote(interpIndex))
	buf.WriteString(`,"_type":`)

	interpType, err = interpolateFlag(e, m, e.Type)
	buf.WriteString(strconv.Quote(interpType))

	//Interpolate the Id flag
	interpId, err = interpolateFlag(e, m, e.Id)

	//Check that Id successfully interpolated. If not then do not specify id
	//at all and default to auto-generated one.
	if len(e.Id) > 0 && err == nil {
		buf.WriteString(`,"_id":`)
		buf.WriteString(strconv.Quote(interpId))
	}
	if e.Timestamp != nil {
		t := time.Unix(0, *e.Timestamp)
		buf.WriteString(`,"_timestamp":"`)
		buf.WriteString(t.Format(e.TimestampFormat))
		buf.WriteString(`"`)
	}
	buf.WriteString(`}}`)
}

// Replaces a date pattern (ex: %{2012.09.19} in the index name
func interpolateFlag(e *ElasticSearchCoordinates, m *message.Message, name string) (
	interpolatedValue string, err error) {

	iSlice := strings.Split(name, "%{")
	var t time.Time

	for i, element := range iSlice {
		elEnd := strings.Index(element, "}")

		if elEnd > -1 {
			elVal := element[:elEnd]
			switch elVal {
			case "Type":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], m.GetType(), -1)
			case "Hostname":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], m.GetHostname(), -1)
			case "Pid":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1],
					strconv.Itoa(int(m.GetPid())), -1)
			case "UUID":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], m.GetUuidString(), -1)
			case "Logger":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], m.GetLogger(), -1)
			case "EnvVersion":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], m.GetEnvVersion(), -1)
			case "Severity":
				iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1],
					strconv.Itoa(int(m.GetSeverity())), -1)
			default:
				if fname, ok := m.GetFieldValue(elVal); ok {
					iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], fname.(string), -1)
				} else {
					if e.ESIndexFromTimestamp && e.Timestamp != nil {
						t = time.Unix(0, *e.Timestamp).UTC()
					} else {
						t = time.Now()
					}
					iSlice[i] = strings.Replace(iSlice[i], element[:elEnd+1], t.Format(elVal), -1)
				}
			}
			if iSlice[i] == elVal {
				err = fmt.Errorf("Could not interpolate field from config: %s", name)
			}
		}
	}
	interpolatedValue = strings.Join(iSlice, "")
	return
}

type ESCoordGenerator struct {
	Index                string
	Type                 string
	TimestampFormat      string
	ESIndexFromTimestamp bool
	Id                   string
}

// Creates an ElasticSearchCoordinates struct, serializes it, and appends the
// result onto the provided byte slice.
func (cgen *ESCoordGenerator) GenerateCoordinates(pack *PipelinePack, buf *bytes.Buffer) {
	coordinates := &ElasticSearchCoordinates{
		Index:                cgen.Index,
		Type:                 cgen.Type,
		Timestamp:            pack.Message.Timestamp,
		TimestampFormat:      cgen.TimestampFormat,
		ESIndexFromTimestamp: cgen.ESIndexFromTimestamp,
		Id:                   cgen.Id,
	}

	coordinates.PopulateBuffer(pack.Message, buf)
}

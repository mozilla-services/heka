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
#   Victor Ng (vng@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package payload

import (
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	. "github.com/mozilla-services/heka/pipeline"
	pipeline_ts "github.com/mozilla-services/heka/pipeline/testsupport"
	"github.com/mozilla-services/heka/pipelinemock"
	"github.com/rafrombrc/gomock/gomock"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"strings"
	"time"
)

func PayloadDecodersSpec(c gospec.Context) {
	t := &pipeline_ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("A PayloadXmlDecoder", func() {
		decoder := new(PayloadXmlDecoder)
		conf := decoder.ConfigStruct().(*PayloadXmlDecoderConfig)
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		c.Specify("decodes simple messages", func() {
			xml_data := `<library>
    <!-- Great book. -->
    <book id="b0836217462" available="true">
        <isbn>0836217462</isbn>
        <title lang="en">Being a Dog Is a Full-Time Job</title>
        <quote>I'd dog paddle the deepest ocean.</quote>
        <author id="CMS">
            <?echo "go rocks"?>
            <name>Charles M Schulz</name>
            <born>1922-11-26</born>
            <dead>2000-02-12</dead>
        </author>
        <character id="PP">
            <name>Peppermint Patty</name>
            <born>1966-08-22</born>
            <qualificati>bold, brash and tomboyish</qualificati>
        </character>
        <character id="Snoopy">
            <name>Snoopy</name>
            <born>1950-10-04</born>
            <qualificati>extroverted beagle</qualificati>
        </character>
    </book>
</library>`

			conf.XPathMapConfig = map[string]string{"Isbn": "library/*/isbn",
				"Name":    "/library/book/character[born='1950-10-04']/name",
				"Patty":   "/library/book//node()[@id='PP']/name",
				"Title":   "//book[author/@id='CMS']/title",
				"Comment": "/library/book/preceding::comment()",
			}

			conf.MessageFields = MessageTemplate{
				"Isbn":    "%Isbn%",
				"Name":    "%Name%",
				"Patty":   "%Patty%",
				"Title":   "%Title%",
				"Comment": "%Comment%",
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(xml_data)
			_, err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			var isbn, name, patty, title, comment interface{}
			var ok bool

			isbn, ok = pack.Message.GetFieldValue("Isbn")
			c.Expect(ok, gs.Equals, true)

			name, ok = pack.Message.GetFieldValue("Name")
			c.Expect(ok, gs.Equals, true)

			patty, ok = pack.Message.GetFieldValue("Patty")
			c.Expect(ok, gs.Equals, true)

			title, ok = pack.Message.GetFieldValue("Title")
			c.Expect(ok, gs.Equals, true)

			comment, ok = pack.Message.GetFieldValue("Comment")
			c.Expect(ok, gs.Equals, true)

			c.Expect(isbn, gs.Equals, "0836217462")
			c.Expect(name, gs.Equals, "Snoopy")
			c.Expect(patty, gs.Equals, "Peppermint Patty")
			c.Expect(title, gs.Equals, "Being a Dog Is a Full-Time Job")
			c.Expect(comment, gs.Equals, " Great book. ")
		})
	})

	c.Specify("A PayloadRegexDecoder", func() {
		decoder := new(PayloadRegexDecoder)
		conf := decoder.ConfigStruct().(*PayloadRegexDecoderConfig)
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)
		conf.TimestampLayout = "02/Jan/2006:15:04:05 -0700"

		c.Specify("non capture regex", func() {
			conf.MatchRegex = `\d+`
			err := decoder.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "PayloadRegexDecoder regex must contain capture groups")
		})

		c.Specify("invalid regex", func() {
			conf.MatchRegex = `\mtest`
			err := decoder.Init(conf)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "PayloadRegexDecoder: error parsing regexp: invalid escape sequence: `\\m`")
		})

		c.Specify("reading an apache timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[18/Apr/2013:14:00:28 -0700]")
			_, err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1366318828000000000))
			pack.Zero()
		})

		c.Specify("logs invalid messages", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("invalid payload")
			_, err = decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
			c.Expect(err.Error(), gs.Equals, "No match: invalid payload")
			pack.Zero()
		})

		c.Specify("ignores invalid messages when log_errors is disabled", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			conf.LogErrors = false
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("invalid payload")
			_, err = decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			pack.Zero()
		})

		c.Specify("uses kitchen timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[5:16PM]")
			now := time.Now()
			cur_date := time.Date(now.Year(), now.Month(), now.Day(), 17, 16, 0, 0,
				time.UTC)
			_, err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, cur_date.UnixNano())
			pack.Zero()
		})

		c.Specify("supports Epoch timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			conf.TimestampLayout = "Epoch"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[1414448234]")
			_, err = decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1414448234000000000))
			pack.Zero()
		})

		c.Specify("supports Epoch timestamp w/ float", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			conf.TimestampLayout = "Epoch"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[1414448234.638504391]")
			_, err = decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1414448234638504391))
			pack.Zero()
		})

		c.Specify("adjusts timestamps as specified", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			conf.TimestampLayout = "02/Jan/2006:15:04:05"
			conf.TimestampLocation = "America/Los_Angeles"
			timeStr := "18/Apr/2013:14:00:28"
			loc, err := time.LoadLocation(conf.TimestampLocation)
			c.Assume(err, gs.IsNil)
			expectedLocal, err := time.ParseInLocation(conf.TimestampLayout, timeStr, loc)
			c.Assume(err, gs.IsNil)
			err = decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[" + timeStr + "]")
			_, err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, expectedLocal.UnixNano())
			pack.Zero()
		})

		c.Specify("apply representation metadata to a captured field", func() {
			value := "0.23"
			payload := "header"
			conf.MatchRegex = `(?P<ResponseTime>\d+\.\d+)`
			conf.MessageFields = MessageTemplate{
				"ResponseTime|s": "%ResponseTime%",
				"Payload|s":      "%ResponseTime%",
				"Payload":        payload,
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(value)
			_, err = decoder.Decode(pack)

			f := pack.Message.FindFirstField("ResponseTime")
			c.Expect(f, gs.Not(gs.IsNil))
			c.Expect(f.GetValue(), gs.Equals, value)
			c.Expect(f.GetRepresentation(), gs.Equals, "s")

			f = pack.Message.FindFirstField("Payload")
			c.Expect(f, gs.Not(gs.IsNil))
			c.Expect(f.GetValue(), gs.Equals, value)
			c.Expect(f.GetRepresentation(), gs.Equals, "s")

			c.Expect(pack.Message.GetPayload(), gs.Equals, payload)

			pack.Zero()
		})

		c.Specify("reading test-zeus.log", func() {
			conf.MatchRegex = `(?P<Ip>([0-9]{1,3}\.){3}[0-9]{1,3}) (?P<Hostname>(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])) (?P<User>\w+) \[(?P<Timestamp>[^\]]+)\] \"(?P<Verb>[A-X]+) (?P<Request>\/\S*) HTTP\/(?P<Httpversion>\d\.\d)\" (?P<Response>\d{3}) (?P<Bytes>\d+)`
			conf.MessageFields = MessageTemplate{
				"hostname": "%Hostname%",
				"ip":       "%Ip%",
				"response": "%Response%",
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			filePath := "../testsupport/test-zeus.log"
			fileBytes, err := ioutil.ReadFile(filePath)
			c.Assume(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")

			containsFieldValue := func(str, fieldName string, msg *message.Message) bool {
				raw, ok := msg.GetFieldValue(fieldName)
				if !ok {
					return false
				}
				value := raw.(string)
				return strings.Contains(str, value)
			}

			c.Specify("extracts capture data and puts it in the message fields", func() {
				var misses int
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					pack.Message.SetPayload(line)
					_, err = decoder.Decode(pack)
					if err != nil {
						misses++
						continue
					}
					c.Expect(containsFieldValue(line, "hostname", pack.Message), gs.IsTrue)
					c.Expect(containsFieldValue(line, "ip", pack.Message), gs.IsTrue)
					c.Expect(containsFieldValue(line, "response", pack.Message), gs.IsTrue)
					pack.Zero()
				}
				c.Expect(misses, gs.Equals, 3)
			})
		})

		c.Specify("reading test-severity.log", func() {
			conf.MatchRegex = `severity: (?P<Severity>[a-zA-Z]+)`
			conf.SeverityMap = map[string]int32{
				"emergency": 0,
				"alert":     1,
				"critical":  2,
				"error":     3,
				"warning":   4,
				"notice":    5,
				"info":      6,
				"debug":     7,
			}
			reverseMap := make(map[int32]string)
			for str, i := range conf.SeverityMap {
				reverseMap[i] = str
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := pipelinemock.NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)

			filePath := "../testsupport/test-severity.log"
			fileBytes, err := ioutil.ReadFile(filePath)
			c.Assume(err, gs.IsNil)
			fileStr := string(fileBytes)
			lines := strings.Split(fileStr, "\n")

			c.Specify("sets message severity based on SeverityMap", func() {
				err := errors.New("Don't recognize severity: 'BOGUS'")
				dRunner.EXPECT().LogError(err)
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					pack.Message.SetPayload(line)
					_, err = decoder.Decode(pack)
					if err != nil {
						fmt.Println(line)
					}
					c.Expect(err, gs.IsNil)
					if strings.Contains(line, "BOGUS") {
						continue
					}
					strVal := reverseMap[pack.Message.GetSeverity()]
					c.Expect(strings.Contains(line, strVal), gs.IsTrue)
				}
			})
		})
	})

}

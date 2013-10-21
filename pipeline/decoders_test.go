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

package pipeline

import (
	"code.google.com/p/gomock/gomock"
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/sandbox"
	ts "github.com/mozilla-services/heka/testsupport"
	"github.com/rafrombrc/gospec/src/gospec"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Attach an `Init` method to MockDecoders so they'll work w/ PluginWrappers
func (d *MockDecoder) Init(config interface{}) (err error) {
	return
}

func DecodersSpec(c gospec.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	msg := getTestMessage()
	config := NewPipelineConfig(nil)

	c.Specify("A ProtobufDecoder", func() {
		encoded, err := proto.Marshal(msg)
		c.Assume(err, gs.IsNil)
		pack := NewPipelinePack(config.inputRecycleChan)
		decoder := new(ProtobufDecoder)

		c.Specify("decodes a protobuf message", func() {
			pack.MsgBytes = encoded
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			c.Expect(pack.Message, gs.Equals, msg)
			v, ok := pack.Message.GetFieldValue("foo")
			c.Expect(ok, gs.IsTrue)
			c.Expect(v, gs.Equals, "bar")
		})

		c.Specify("returns an error for bunk encoding", func() {
			bunk := append([]byte{0, 0, 0}, encoded...)
			pack.MsgBytes = bunk
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
		})
	})

	c.Specify("A MultiDecoder", func() {
		decoder := new(MultiDecoder)
		conf := decoder.ConfigStruct().(*MultiDecoderConfig)

		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		conf.Name = "MyMultiDecoder"
		conf.Subs = make(map[string]interface{}, 0)

		conf.Subs["StartsWithM"] = make(map[string]interface{}, 0)
		withM := conf.Subs["StartsWithM"].(map[string]interface{})
		withM["type"] = "PayloadRegexDecoder"
		withM["match_regex"] = "^(?P<TheData>m.*)"
		withMFields := make(map[string]interface{}, 0)
		withMFields["StartsWithM"] = "%TheData%"
		withM["message_fields"] = withMFields

		conf.Order = []string{"StartsWithM"}

		errMsg := "Unable to decode message with any contained decoder."

		dRunner := NewMockDecoderRunner(ctrl)
		// An error will be spit out b/c there's no real *dRunner in there;
		// doesn't impact the tests.
		dRunner.EXPECT().LogError(gomock.Any())

		c.Specify("decodes simple messages", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "matching text"
			pack.Message.SetPayload(regex_data)
			err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetType(), gs.Equals, "heka.MyMultiDecoder")
			value, ok := pack.Message.GetFieldValue("StartsWithM")
			c.Assume(ok, gs.IsTrue)
			c.Expect(value, gs.Equals, regex_data)
		})

		c.Specify("returns an error if all decoders fail", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "non-matching text"
			pack.Message.SetPayload(regex_data)
			err = decoder.Decode(pack)
			c.Expect(err.Error(), gs.Equals, errMsg)
		})

		c.Specify("logs subdecoder failures when configured to do so", func() {
			conf.LogSubErrors = true
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			decoder.SetDecoderRunner(dRunner)
			regex_data := "non-matching text"
			pack.Message.SetPayload(regex_data)

			// Expect that we log an error for undecoded message.
			dRunner.EXPECT().LogError(fmt.Errorf("Subdecoder 'StartsWithM' decode error: No match"))

			err = decoder.Decode(pack)
			c.Expect(err.Error(), gs.Equals, errMsg)
		})

		c.Specify("sets subdecoder runner correctly", func() {
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)

			// Call LogError to appease the angry gomock gods.
			dRunner.LogError(errors.New("foo"))

			// Now create a real *dRunner, pass it in, make sure a wrapper
			// gets handed to the subdecoder.
			dr := NewDecoderRunner(decoder.Name, decoder, new(PluginGlobals))
			decoder.SetDecoderRunner(dr)
			sub := decoder.Decoders["StartsWithM"]
			subRunner := sub.(*PayloadRegexDecoder).dRunner
			subRunner, ok := subRunner.(*mDRunner)
			c.Expect(ok, gs.IsTrue)
			c.Expect(subRunner.Name(), gs.Equals,
				fmt.Sprintf("%s-StartsWithM", decoder.Name))
			c.Expect(subRunner.Decoder(), gs.Equals, sub)
		})

		c.Specify("with multiple registered decoders", func() {
			conf.Subs["StartsWithS"] = make(map[string]interface{}, 0)
			withS := conf.Subs["StartsWithS"].(map[string]interface{})
			withS["type"] = "PayloadRegexDecoder"
			withS["match_regex"] = "^(?P<TheData>s.*)"
			withSFields := make(map[string]interface{}, 0)
			withSFields["StartsWithS"] = "%TheData%"
			withS["message_fields"] = withSFields

			conf.Subs["StartsWithM2"] = make(map[string]interface{}, 0)
			withM2 := conf.Subs["StartsWithM2"].(map[string]interface{})
			withM2["type"] = "PayloadRegexDecoder"
			withM2["match_regex"] = "^(?P<TheData>m.*)"
			withM2Fields := make(map[string]interface{}, 0)
			withM2Fields["StartsWithM2"] = "%TheData%"
			withM2["message_fields"] = withM2Fields

			conf.Order = append(conf.Order, "StartsWithS", "StartsWithM2")

			var ok bool

			// Two more subdecoders means two more LogError calls.
			dRunner.EXPECT().LogError(gomock.Any()).Times(2)

			c.Specify("defaults to `first-wins` cascading", func() {
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)

				c.Specify("on a first match condition", func() {
					regexData := "match first"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("and a second match condition", func() {
					regexData := "second match"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("returning an error if they all fail", func() {
					regexData := "won't match"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err.Error(), gs.Equals, errMsg)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})
			})

			c.Specify("and using `all` cascading", func() {
				conf.CascadeStrategy = "all"
				err := decoder.Init(conf)
				c.Assume(err, gs.IsNil)
				decoder.SetDecoderRunner(dRunner)

				c.Specify("matches multiples when appropriate", func() {
					regexData := "matches twice"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsTrue)
				})

				c.Specify("matches singles when appropriate", func() {
					regexData := "second match"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err, gs.IsNil)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsTrue)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})

				c.Specify("returns an error if they all fail", func() {
					regexData := "won't match"
					pack.Message.SetPayload(regexData)
					err = decoder.Decode(pack)
					c.Expect(err.Error(), gs.Equals, errMsg)
					_, ok = pack.Message.GetFieldValue("StartsWithM")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithS")
					c.Expect(ok, gs.IsFalse)
					_, ok = pack.Message.GetFieldValue("StartsWithM2")
					c.Expect(ok, gs.IsFalse)
				})
			})
		})
	})

	c.Specify("A PayloadJsonDecoder", func() {
		decoder := new(PayloadJsonDecoder)
		conf := decoder.ConfigStruct().(*PayloadJsonDecoderConfig)
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		c.Specify("decodes simple messages", func() {
			json_data := `{"statsd": {"count": 1, "name": "some.counter"}, "pid": 532, "timestamp": "2013-08-13T10:32:00.000Z"}`
			conf.JsonMap = map[string]string{"Count": "$.statsd.count",
				"Name":      "$.statsd.name",
				"Pid":       "$.pid",
				"Timestamp": "$.timestamp",
			}

			conf.MessageFields = MessageTemplate{
				"Pid":       "%Pid%",
				"StatCount": "%Count%",
				"StatName":  "%Name%",
				"Timestamp": "%Timestamp%",
			}
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(json_data)
			err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)
			c.Expect(pack.Message.GetPid(), gs.Equals, int32(532))

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1376389920000000000))

			var ok bool
			var name, count interface{}
			count, ok = pack.Message.GetFieldValue("StatCount")
			c.Expect(ok, gs.Equals, true)
			c.Expect(count, gs.Equals, "1")

			name, ok = pack.Message.GetFieldValue("StatName")
			c.Expect(ok, gs.Equals, true)
			c.Expect(name, gs.Equals, "some.counter")
		})
	})

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
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(xml_data)
			err = decoder.Decode(pack)
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
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[18/Apr/2013:14:00:28 -0700]")
			err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, int64(1366318828000000000))
			pack.Zero()
		})

		c.Specify("uses kitchen timestamp", func() {
			conf.MatchRegex = `\[(?P<Timestamp>[^\]]+)\]`
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[5:16PM]")
			now := time.Now()
			cur_date := time.Date(now.Year(), now.Month(), now.Day(), 17, 16, 0, 0,
				time.UTC)
			err = decoder.Decode(pack)
			c.Expect(pack.Message.GetTimestamp(), gs.Equals, cur_date.UnixNano())
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
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload("[" + timeStr + "]")
			err = decoder.Decode(pack)
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
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(value)
			err = decoder.Decode(pack)

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
					err = decoder.Decode(pack)
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
			dRunner := NewMockDecoderRunner(ctrl)
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
					err = decoder.Decode(pack)
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

	c.Specify("A SandboxDecoder", func() {
		decoder := new(SandboxDecoder)
		conf := decoder.ConfigStruct().(*sandbox.SandboxConfig)
		conf.ScriptFilename = "../sandbox/lua/testsupport/decoder.lua"
		conf.ScriptType = "lua"
		supply := make(chan *PipelinePack, 1)
		pack := NewPipelinePack(supply)

		c.Specify("decodes simple messages", func() {
			data := "1376389920 debug id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			err = decoder.Decode(pack)
			c.Assume(err, gs.IsNil)

			c.Expect(pack.Message.GetTimestamp(),
				gs.Equals,
				int64(1376389920000000000))

			c.Expect(pack.Message.GetSeverity(), gs.Equals, int32(7))

			var ok bool
			var value interface{}
			value, ok = pack.Message.GetFieldValue("id")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "2321")

			value, ok = pack.Message.GetFieldValue("url")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "example.com")

			value, ok = pack.Message.GetFieldValue("item")
			c.Expect(ok, gs.Equals, true)
			c.Expect(value, gs.Equals, "1")
		})

		c.Specify("decodes an invalid messages", func() {
			data := "1376389920 bogus id=2321 url=example.com item=1"
			err := decoder.Init(conf)
			c.Assume(err, gs.IsNil)
			dRunner := NewMockDecoderRunner(ctrl)
			decoder.SetDecoderRunner(dRunner)
			pack.Message.SetPayload(data)
			err = decoder.Decode(pack)
			c.Expect(err.Error(), gs.Equals, "Failed parsing: "+data)
			c.Expect(decoder.processMessageFailures, gs.Equals, int64(1))
		})
	})

	c.Specify("A StatsToFieldsDecoder", func() {
		decoder := new(StatsToFieldsDecoder)
		router := NewMessageRouter()
		router.inChan = make(chan *PipelinePack, 5)
		dRunner := NewMockDecoderRunner(ctrl)
		decoder.runner = dRunner
		dRunner.EXPECT().Router().Return(router)

		pack := NewPipelinePack(config.inputRecycleChan)

		mergeStats := func(stats [][]string) string {
			lines := make([]string, len(stats))
			for i, line := range stats {
				lines[i] = strings.Join(line, " ")
			}
			return strings.Join(lines, "\n")
		}

		c.Specify("correctly converts stats to fields", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047333"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)

			for i, stats := range stats {
				value, ok := pack.Message.GetFieldValue(stats[0])
				c.Expect(ok, gs.IsTrue)
				expected := float64(i + 1)
				c.Expect(value.(float64), gs.Equals, expected)
			}

			value, ok := pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err := strconv.ParseInt(stats[0][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)
		})

		c.Specify("generates multiple messages for multiple timestamps", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047331"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			// Prime the pack supply w/ two new packs.
			dRunner.EXPECT().NewPack().Return(NewPipelinePack(nil))
			dRunner.EXPECT().NewPack().Return(NewPipelinePack(nil))

			// Decode and check the main pack.
			pack.Message.SetPayload(mergeStats(stats))
			err := decoder.Decode(pack)
			c.Expect(err, gs.IsNil)
			value, ok := pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err := strconv.ParseInt(stats[0][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)

			// Check the first extra.
			pack = <-router.inChan
			value, ok = pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err = strconv.ParseInt(stats[2][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)

			// Check the second extra.
			pack = <-router.inChan
			value, ok = pack.Message.GetFieldValue("timestamp")
			c.Expect(ok, gs.IsTrue)
			expected, err = strconv.ParseInt(stats[4][2], 0, 32)
			c.Assume(err, gs.IsNil)
			c.Expect(value.(int64), gs.Equals, expected)
		})

		c.Specify("fails w/ invalid timestamp", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333c"},
				{"stat.four", "4", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := fmt.Sprintf("invalid timestamp: '%s'",
				strings.Join(stats[2], " "))
			c.Expect(err.Error(), gs.Equals, expected)
		})

		c.Specify("fails w/ invalid value", func() {
			stats := [][]string{
				{"stat.one", "1", "1380047333"},
				{"stat.two", "2", "1380047333"},
				{"stat.three", "3", "1380047333"},
				{"stat.four", "4d", "1380047333"},
				{"stat.five", "5", "1380047332"},
			}
			pack.Message.SetPayload(mergeStats(stats))
			err := decoder.Decode(pack)
			c.Expect(err, gs.Not(gs.IsNil))
			expected := fmt.Sprintf("invalid value: '%s'",
				strings.Join(stats[3], " "))
			c.Expect(err.Error(), gs.Equals, expected)
		})
	})
}

func BenchmarkEncodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		proto.Marshal(msg)
	}
}

func BenchmarkDecodeProtobuf(b *testing.B) {
	b.StopTimer()
	msg := getTestMessage()
	encoded, _ := proto.Marshal(msg)
	config := NewPipelineConfig(nil)
	pack := NewPipelinePack(config.inputRecycleChan)
	decoder := new(ProtobufDecoder)
	pack.MsgBytes = encoded
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		decoder.Decode(pack)
	}
}

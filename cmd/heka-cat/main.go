/***** BEGIN LICENSE BLOCK *****
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this file,
# You can obtain one at http://mozilla.org/MPL/2.0/.
#
# The Initial Developer of the Original Code is the Mozilla Foundation.
# Portions created by the Initial Developer are Copyright (C) 2014
# the Initial Developer. All Rights Reserved.
#
# Contributor(s):
#   Mike Trinkala (trink@mozilla.com)
# ***** END LICENSE BLOCK *****/

/*

A command-line utility for counting, viewing, filtering, and extracting Heka
protobuf logs.

*/
package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"github.com/mozilla-services/heka/pipeline"
	"io"
	"os"
	"time"
)

func main() {
	flagMatch := flag.String("match", "TRUE", "message_matcher filter expression")
	flagFormat := flag.String("format", "txt", "output format [txt|json|heka|count]")
	flagOutput := flag.String("output", "", "output filename, defaults to stdout")
	flagTail := flag.Bool("tail", false, "don't exit on EOF")
	flagOffset := flag.Int64("offset", 0, "starting offset for the input file in bytes")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	var err error
	var match *message.MatcherSpecification
	if match, err = message.CreateMatcherSpecification(*flagMatch); err != nil {
		fmt.Printf("Match specification - %s\n", err)
		os.Exit(2)
	}

	var file *os.File
	if file, err = os.Open(flag.Arg(0)); err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(3)
	}
	defer file.Close()

	var out *os.File
	if "" == *flagOutput {
		out = os.Stdout
	} else {
		if out, err = os.OpenFile(*flagOutput, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			fmt.Printf("%s\n", err)
			os.Exit(4)
		}
		defer out.Close()
	}

	var offset int64
	if offset, err = file.Seek(*flagOffset, 0); err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(5)
	}

	parser := pipeline.NewMessageProtoParser()
	msg := new(message.Message)
	var processed, matched int64

	fmt.Printf("Input:%s  Offset:%d  Match:%s  Format:%s  Tail:%t  Output:%s\n",
		flag.Arg(0), *flagOffset, *flagMatch, *flagFormat, *flagTail, *flagOutput)
	for true {
		n, record, err := parser.Parse(file)
		if n > 0 && n != len(record) {
			fmt.Printf("Corruption detected at offset: %d bytes: %d\n", offset, n-len(record))
		}
		if err != nil {
			if err == io.EOF {
				if !*flagTail || "count" == *flagFormat {
					break
				}
				time.Sleep(time.Duration(500) * time.Millisecond)
			} else {
				break
			}
		} else {
			if len(record) > 0 {
				processed += 1
				headerLen := int(record[1]) + message.HEADER_FRAMING_SIZE
				if err = proto.Unmarshal(record[headerLen:], msg); err != nil {
					fmt.Printf("Error unmarshalling message at offset: %d error: %s\n", offset, err)
					continue
				}

				if !match.Match(msg) {
					continue
				}
				matched += 1

				switch *flagFormat {
				case "count":
					// no op
				case "json":
					contents, _ := json.Marshal(msg)
					fmt.Fprintf(out, "%s\n", contents)
				case "heka":
					fmt.Fprintf(out, "%s", record)
				default:
					fmt.Fprintf(out, "Timestamp: %s\n"+
						"Type: %s\n"+
						"Hostname: %s\n"+
						"Pid: %d\n"+
						"UUID: %s\n"+
						"Logger: %s\n"+
						"Payload: %s\n"+
						"EnvVersion: %s\n"+
						"Severity: %d\n"+
						"Fields: %+v\n\n",
						time.Unix(0, msg.GetTimestamp()), msg.GetType(),
						msg.GetHostname(), msg.GetPid(), msg.GetUuidString(),
						msg.GetLogger(), msg.GetPayload(), msg.GetEnvVersion(),
						msg.GetSeverity(), msg.Fields)
				}
			}
		}
		offset += int64(n)
	}
	fmt.Printf("Processed: %d, matched: %d messages\n", processed, matched)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(6)
	}
}

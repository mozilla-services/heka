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

package pipeline

import (
	"fmt"
	"strconv"
)

// Supports int, float, and string values
type PluginReport map[string]interface{}

// Throws out any value that can't be converted to either int64, float64, or
// string.
func (pr PluginReport) Stringify() (prStr map[string]string) {
	prStr = make(map[string]string)
	var (
		ok bool
		i  int64
		f  float64
		s  string
	)
	for k, v := range pr {
		if i, ok = v.(int64); ok {
			prStr[k] = strconv.FormatInt(i, 10)
		} else if f, ok = v.(float64); ok {
			prStr[k] = strconv.FormatFloat(f, 'g', 6, 64)
		} else if s, ok = v.(string); ok {
			prStr[k] = s
		}
	}
	return
}

// Interface for Heka plugins that will provide data for the queue report.
type ReportingPlugin interface {
	ReportData() PluginReport
}

type chanStat struct {
	chanName string
	capacity int
	length   int
}

// Generate and return queue (i.e. channel) reports and plugin reports.
func (pc *PipelineConfig) reports() (qReports map[string][]chanStat,
	pReports map[string]PluginReport) {

	qReports = make(map[string][]chanStat)
	pReports = make(map[string]PluginReport)

	recycleReport := make([]chanStat, 1)
	recycleReport[0] = chanStat{
		"RecycleChan", cap(pc.RecycleChan), len(pc.RecycleChan)}
	qReports["RecycleChan"] = recycleReport

	var (
		stat     chanStat
		reporter ReportingPlugin
		ok       bool
	)

	for name, runner := range pc.InputRunners {
		if reporter, ok = runner.Plugin().(ReportingPlugin); ok {
			pReports[name] = reporter.ReportData()
		}
	}

	fqReport := make([]chanStat, 0, len(pc.FilterRunners))
	for name, runner := range pc.FilterRunners {
		stat = chanStat{name, cap(runner.InChan()), len(runner.InChan())}
		fqReport = append(fqReport, stat)
		if reporter, ok = runner.Plugin().(ReportingPlugin); ok {
			pReports[name] = reporter.ReportData()
		}
	}
	qReports["filter-queues"] = fqReport

	oqReport := make([]chanStat, 0, len(pc.OutputRunners))
	for name, runner := range pc.OutputRunners {
		stat = chanStat{name, cap(runner.InChan()), len(runner.InChan())}
		oqReport = append(oqReport, stat)
		if reporter, ok = runner.Plugin().(ReportingPlugin); ok {
			pReports[name] = reporter.ReportData()
		}
	}
	qReports["output-queues"] = oqReport

	var reportName string
	for i, dSet := range pc.decoderRunners {
		dSetReport := make([]chanStat, len(dSet))
		for j, runner := range dSet {
			stat = chanStat{runner.Name(), cap(runner.InChan()), len(runner.InChan())}
			dSetReport[j] = stat
		}
		reportName = fmt.Sprintf("decoder-set-queues-%d", i)
		qReports[reportName] = dSetReport
	}

	return
}

func (pc *PipelineConfig) sendReport() {
	qReports, pReports := pc.reports()
	var (
		stat                chanStat
		payload, name, k, v string
		qr                  []chanStat
		pr                  PluginReport
	)

	for name, qr = range qReports {
		payload = fmt.Sprintf("%s%s:\n", payload, name)
		for _, stat = range qr {
			payload = fmt.Sprintf("%s\t%s:\t%d\t%d\n", payload, stat.chanName,
				stat.capacity, stat.length)
		}
	}

	for name, pr = range pReports {
		payload = fmt.Sprintf("%s%s:\n", payload, name)
		for k, v = range pr.Stringify() {
			payload = fmt.Sprintf("%s:\t%s\n", k, v)
		}
	}

	msg := MessageGenerator.Retrieve()
	msg.Message.SetType("heka.queue-report")
	msg.Message.SetPayload(payload)
	MessageGenerator.Inject(msg)
}

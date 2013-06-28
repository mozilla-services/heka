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
#   Mike Trinkala (trink@mozilla.com)
#
# ***** END LICENSE BLOCK *****/

package pipeline

import (
	"encoding/json"
	"fmt"
	"github.com/mozilla-services/heka/message"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type DashboardOutputConfig struct {
	// IP address of the Dashboard HTTP interface (defaults to all interfaces on
	// port 4352 (HEKA))
	Address string `toml:"address"`
	// Working directory where the Dashboard output is written to; it also
	// serves as the root for the HTTP fileserver.  This directory is created
	// if necessary and if it exists the previous output is wiped clean.
	// *DO NOT* store any user created content here.
	WorkingDirectory string `toml:"working_directory"`
	// Default interval at which dashboard will update is 5 seconds.
	TickerInterval uint `toml:"ticker_interval"`
	// Default message matcher
	MessageMatcher string
}

func (self *DashboardOutput) ConfigStruct() interface{} {
	return &DashboardOutputConfig{
		Address:          ":4352",
		WorkingDirectory: "./dashboard",
		TickerInterval:   uint(5),
		MessageMatcher:   "Type == 'heka.all-report' || Type == 'heka.sandbox-terminated' || Type == 'heka.sandbox-output'",
	}
}

type DashboardOutput struct {
	workingDirectory string
	server           *http.Server
}

func (self *DashboardOutput) Init(config interface{}) (err error) {
	conf := config.(*DashboardOutputConfig)

	self.workingDirectory, _ = filepath.Abs(conf.WorkingDirectory)
	if err = os.MkdirAll(self.workingDirectory, 0700); err != nil {
		return fmt.Errorf("Can't create the working directory for the dashboard output: %s", err.Error())
	}

	for _, fname := range []string{"heka_report.html", "heka_sandbox_termination.html", "heka.js"} {
		if err = overwriteFile(path.Join(self.workingDirectory, fname), "ok"); err != nil {
			return
		}
	}

	// delete all previous output
	if matches, err := filepath.Glob(path.Join(self.workingDirectory, "*.*")); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}
	overwriteFile(path.Join(self.workingDirectory, "heka_report.html"), getReportHtml())
	overwriteFile(path.Join(self.workingDirectory, "heka_sandbox_termination.html"), getSandboxTerminationHtml())
	overwriteFile(path.Join(self.workingDirectory, "heka.js"), getHekaJs())

	h := http.FileServer(http.Dir(self.workingDirectory))
	http.Handle("/", h)
	self.server = &http.Server{
		Addr:         conf.Address,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go self.server.ListenAndServe()

	return
}

func (self *DashboardOutput) Run(or OutputRunner, h PluginHelper) (err error) {
	inChan := or.InChan()
	ticker := or.Ticker()

	var (
		ok   = true
		pack *PipelinePack
		msg  *message.Message
	)

	reNotWord, _ := regexp.Compile("\\W")
	for ok {
		select {
		case pack, ok = <-inChan:
			if !ok {
				break
			}
			msg = pack.Message
			switch msg.GetType() {
			case "heka.all-report":
				fn := path.Join(self.workingDirectory, "heka_report.json")
				createPluginPages(self.workingDirectory, msg.GetPayload())
				overwriteFile(fn, msg.GetPayload())
			case "heka.sandbox-output":
				tmp, _ := msg.GetFieldValue("payload_type")
				if payloadType, ok := tmp.(string); ok {
					var payloadName, nameExt string
					tmp, _ := msg.GetFieldValue("payload_name")
					if payloadName, ok = tmp.(string); ok {
						nameExt = reNotWord.ReplaceAllString(payloadName, "")
					}
					if len(nameExt) > 64 {
						nameExt = nameExt[:64]
					}
					nameExt = "." + nameExt

					payloadType = reNotWord.ReplaceAllString(payloadType, "")
					fn := msg.GetLogger() + nameExt + "." + payloadType
					ofn := path.Join(self.workingDirectory, fn)
					if payloadType == "cbuf" {
						html := msg.GetLogger() + nameExt + ".html"
						ohtml := path.Join(self.workingDirectory, html)
						_, err := os.Stat(ohtml)
						if err != nil {
							overwriteFile(ohtml, fmt.Sprintf(getCbufTemplate(),
								msg.GetLogger(),
								payloadName,
								fn))
						}
						overwriteFile(ofn, msg.GetPayload())
						updatePluginMetadata(self.workingDirectory, msg.GetLogger(), html, payloadName)
					} else {
						overwriteFile(ofn, msg.GetPayload())
						updatePluginMetadata(self.workingDirectory, msg.GetLogger(), fn, payloadName)
					}
				}
			case "heka.sandbox-terminated":
				fn := path.Join(self.workingDirectory, "heka_sandbox_termination.tsv")
				if file, err := os.OpenFile(fn, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644); err == nil {
					var line string
					if _, ok := msg.GetFieldValue("ProcessMessageCount"); !ok {
						line = fmt.Sprintf("%d\t%s\t%v\n", msg.GetTimestamp()/1e9, msg.GetLogger(), msg.GetPayload())
					} else {
						pmc, _ := msg.GetFieldValue("ProcessMessageCount")
						pms, _ := msg.GetFieldValue("ProcessMessageSamples")
						pmd, _ := msg.GetFieldValue("ProcessMessageAvgDuration")
						ms, _ := msg.GetFieldValue("MatchSamples")
						mad, _ := msg.GetFieldValue("MatchAvgDuration")
						fcl, _ := msg.GetFieldValue("FilterChanLength")
						mcl, _ := msg.GetFieldValue("MatchChanLength")
						rcl, _ := msg.GetFieldValue("RouterChanLength")
						line = fmt.Sprintf("%d\t%s\t%v"+
							" ProcessMessageCount:%v"+
							" ProcessMessageSamples:%v"+
							" ProcessMessageAvgDuration:%v"+
							" MatchSamples:%v"+
							" MatchAvgDuration:%v"+
							" FilterChanLength:%v"+
							" MatchChanLength:%v"+
							" RouterChanLength:%v\n",
							msg.GetTimestamp()/1e9,
							msg.GetLogger(), msg.GetPayload(), pmc, pms, pmd,
							ms, mad, fcl, mcl, rcl)
					}
					file.WriteString(line)
					file.Close()
				}
			}
			pack.Recycle()
		case <-ticker:
			go h.PipelineConfig().allReportsMsg()
		}
	}
	return
}

func overwriteFile(filename, s string) (err error) {
	var file *os.File
	if file, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC+os.O_CREATE, 0644); err == nil {
		file.WriteString(s)
		file.Close()
	}
	return err
}

type PluginOutput struct {
	Filename string
	Name     string
}

type PluginMetadata struct {
	Outputs []PluginOutput
}

func getPluginMetadataPath(dir, logger string) string {
	return path.Join(dir, logger+".json")
}

func updatePluginMetadata(dir, logger, fn, name string) {
	pimd := getPluginMetadata(dir, logger)
	if pimd == nil {
		pimd = new(PluginMetadata)
	}
	found := false
	for _, v := range pimd.Outputs {
		if v.Filename == fn {
			found = true
			break
		}
	}
	if !found {
		pout := PluginOutput{Filename: fn, Name: name}
		pimd.Outputs = append(pimd.Outputs, pout)
		writePluginMetadata(dir, logger, pimd)
	}
}

func writePluginMetadata(dir, logger string, pimd *PluginMetadata) {
	fn := getPluginMetadataPath(dir, logger)
	if file, err := os.OpenFile(fn, os.O_WRONLY|os.O_TRUNC+os.O_CREATE, 0644); err == nil {
		enc := json.NewEncoder(file)
		enc.Encode(pimd)
		file.Close()
	}
}

func getPluginMetadata(dir, logger string) *PluginMetadata {
	var pimd *PluginMetadata
	fn := getPluginMetadataPath(dir, logger)
	if file, err := os.Open(fn); err == nil {
		pimd = new(PluginMetadata)
		dec := json.NewDecoder(file)
		dec.Decode(pimd)
		file.Close()
	}
	return pimd
}

func createOutputTable(dir, logger string) (table string) {
	pimd := getPluginMetadata(dir, logger)
	if pimd == nil {
		return
	}

	outputs := make([]string, 0, 1)
	for _, v := range pimd.Outputs {
		if len(v.Name) == 0 {
			v.Name = "- none -"
		}
		outputs = append(outputs, fmt.Sprintf("<tr><td><a href=\"%s\">%s</a></td><td>%s</td></tr>",
			v.Filename,
			v.Name,
			path.Ext(v.Filename)))
	}
	sort.Strings(outputs)
	table = fmt.Sprintf("<table class=\"outputs\"><caption>Plugin Outputs</caption>"+
		"<thead><tr><th>Name</th><th>Type</th></tr></thead>"+
		"<tbody>\n%s\n</tbody></table>",
		strings.Join(outputs, "\n"))
	return
}

func createPluginPages(dir, payload string) {
	var (
		f      interface{}
		r      []interface{}
		m, p   map[string]interface{}
		ok     bool
		logger string
	)
	if err := json.Unmarshal([]byte(payload), &f); err != nil {
		return
	}
	if m, ok = f.(map[string]interface{}); !ok {
		return
	}
	if r, ok = m["reports"].([]interface{}); !ok {
		return
	}
	for _, plugin := range r {
		if p, ok = plugin.(map[string]interface{}); !ok {
			continue
		}
		if logger, ok = p["Plugin"].(string); !ok {
			continue
		}
		fn := path.Join(dir, logger+".html")
		props := make([]string, 0, 5)
		for k, v := range p {
			mv, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			props = append(props, fmt.Sprintf("<tr><td>%s</td><td>%v</td><td>%v</td></tr>",
				k,
				mv["value"],
				mv["representation"]))
		}
		sort.Strings(props)
		ptable := fmt.Sprintf("<table class=\"properties\"><caption>Properties</caption>"+
			"<thead><tr><th>Name</th><th>Value</th><th>Representation</th></tr></thead>"+
			"<tbody>\n%s\n</tbody></table>",
			strings.Join(props, "\n"))
		otable := createOutputTable(dir, logger)
		overwriteFile(fn, fmt.Sprintf(getPluginTemplate(), logger, ptable, otable))
	}
}

// TODO make the JS libraries part of the local deployment the HTML has them wired up to public web sites
func getReportHtml() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Heka Plugin Report</title>
    <script src="http://yui.yahooapis.com/3.9.1/build/yui/yui-min.js">
    </script>
</head>
<body class="yui3-skin-sam" style="font-size:.8em">
    <div id="report"></div>
<script>
YUI().use("datatable-base", "datasource", "datasource-jsonschema", "datatable-datasource", "datatable-sort", "datatype", function (Y) {
var dataSource = new Y.DataSource.IO({source:"heka_report.json"});
dataSource.plug({fn: Y.Plugin.DataSourceJSONSchema, cfg: {
        schema: {
            resultListLocator: 'reports',
            resultFields: [
                'Plugin',
                {key:'InChanCapacity',locator:'InChanCapacity.value',parser:'number'},
                {key:'InChanLength',locator:'InChanLength.value',parser:'number'},
                {key:'MatchChanCapacity',locator:'MatchChanCapacity.value',parser:'number'},
                {key:'MatchChanLength',locator:'MatchChanLength.value',parser:'number'},
                {key:'MatchAvgDuration',locator:'MatchAvgDuration.value',parser:'number'},
                {key:'ProcessMessageCount',locator:'ProcessMessageCount.value',parser:'number'},
                {key:'InjectMessageCount',locator:'InjectMessageCount.value',parser:'number'}
            ]
        }}
    });

var table = new Y.DataTable({
    columns: [{key: 'Plugin', sortable:true, formatter: '<a href="./{value}.html">{value}</a>', allowHTML: false},
              {key: 'InChanCapacity', sortable:true},
              {key: 'InChanLength', sortable:true},
              {key: 'MatchChanCapacity', sortable:true},
              {key: 'MatchChanLength', sortable:true},
              {key: 'MatchAvgDuration', sortable:true, label: 'MatchAvgDuration (ns)'},
              {key:'ProcessMessageCount', sortable:true, label: 'ProcessedMsgs'},
              {key:'InjectMessageCount', sortable:true, label: 'InjectedMsgs'}
              ],
    caption: 'Heka Plugin Report<br/>(cannot find it? see: <a href="heka_sandbox_termination.html">Heka Sandbox Termination Report</a>)'
});
table.plug(Y.Plugin.DataTableDataSource, {datasource: dataSource})
table.render('#report');
table.datasource.load();
});
</script>
</body>
</html>`
}

func getSandboxTerminationHtml() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Heka Sandbox Termination Report</title>
    <script src="http://yui.yahooapis.com/3.9.1/build/yui/yui-min.js">
    </script>
</head>
<body class="yui3-skin-sam">
    <div id="report"></div>
<script>
function parseTimet(o){
    return new Date(parseInt(o)*1000);
}

YUI().use('datatable-base', 'datasource', 'datasource-textschema', 'datatable-datasource', 'datatable-sort', 'datatable-formatters', 'datatype-date', function (Y) {
var dataSource = new Y.DataSource.IO({source:'heka_sandbox_termination.tsv'});
dataSource.plug({fn: Y.Plugin.DataSourceTextSchema, cfg: {
        schema: {
            resultDelimiter: '\n',
            fieldDelimiter: '\t',
            resultFields: [{key:'Date', parser:parseTimet}, {key:'Plugin'}, {key:'Error Message'}]
        }}
    });

var table = new Y.DataTable({
    columns: [{key: 'Date', formatter:'date', dateFormat:'%D %T', sortable:true},
              {key: 'Plugin', sortable:true},
              {key: 'Error Message', sortable:true}
              ],
    caption: 'Heka Sandbox Termination Report'
});
table.plug(Y.Plugin.DataTableDataSource, {datasource: dataSource})
table.render('#report');
table.datasource.load();
});
</script>
</body>
</html>`
}

func getCbufTemplate() string {
	return `<!DOCTYPE html>
<html>
<head>
    <script src="http://people.mozilla.org/~mtrinkala/heka/dygraph-combined.js"  type="text/javascript">
    </script>
    <script src="heka.js"  type="text/javascript">
    </script>
    <script type="text/javascript">

    function load_complete(cbuf) {
        var name = "graph";
        var plural = "";
        if ((cbuf.header.seconds_per_row * cbuf.header.rows) / 3600 > 1) {
            plural = "s";
        }
        document.getElementById('title').innerHTML = "%s [%s]<br/>"
            + cbuf.header.seconds_per_row + " second aggregation for the last "
            + String((cbuf.header.seconds_per_row * cbuf.header.rows) / 3600) + " hour" + plural;
        var labels = ['Date'];
        for (var i = 0; i < cbuf.header.columns; i++) {
            labels.push(cbuf.header.column_info[i].name + " (" + cbuf.header.column_info[i].unit + ")");
        }

        var checkboxes = document.createElement('div');
        checkboxes.id = name + "_checkboxes";
        var div = document.createElement('div');
        div.id = name;
        div.setAttribute("style","width: 100%%");
        document.body.appendChild(div);
        document.body.appendChild(document.createElement('br'));
        var ldv = cbuf.header.column_info.length * 200 + 150;
        if (ldv > 1024) ldv = 1024;
        var options = {labels: labels, labelsDivWidth: ldv, labelsDivStyles:{ 'textAlign': 'right'}};
        document.body.appendChild(checkboxes);
        graph = new Dygraph(div, cbuf.data, options);
        var colors = graph.getColors();
        for (var i = 1; i < graph.attr_("labels").length; i++) {
            var color = colors[i-1];
            checkboxes.innerHTML += '<input type="checkbox" id="' + (i-1).toString()
            + '" onClick="' + name
            + '.setVisibility(this.id, this.checked)" checked><label style="font-size: smaller; color: '
            + color + '">'+ graph.attr_("labels")[i] + '</label>&nbsp;';
        }
        checkboxes.innerHTML += '<br/><input type="checkbox" id="logscale" onClick="graph.updateOptions({ logscale: this.checked })">'
            + '<label style="font-size: smaller;">Log scale</label>';
        if (cbuf.annotations.length > 0) {
            for (var i = 0; i < cbuf.annotations.length; i++) {
                cbuf.annotations[i].series = labels[cbuf.annotations[i].col];
            }
            graph.setAnnotations(cbuf.annotations);
        }
    }
    </script>
</head>
<body onload="heka_load_cbuf('%s', load_complete);">
<a href="heka_report.html">Dashboard</a>
<p id="title" style="text-align: center">
</p>
</body>
</html>`
}

func getPluginTemplate() string {
	return `<!DOCTYPE html>
<html>
<head>
<style>
body {}
table {border: 1px solid black; float:left; margin-right:50px}
td, th {padding:1px}
#table_container {width:90%%; margin:0 auto}
.outputs {width:250px}
th { background-color: #eee; }
tr:nth-child(even) { background-color:#EDF5FF; }
tr:nth-child(odd) { background-color:#fff; }
</style>
</head>
<body>
<a href="heka_report.html">Dashboard</a>
<p id="title" style="text-align: center; font-weight:bold">%s</p>
<hr/>
<div id="table_container">
<div id="properties">%s</div>
<div id="outputs">%s</div>
</div>
</body>
</html>`
}

func getHekaJs() string {
	return `/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

function heka_parse_cbuf(data) {
    var start = 1;
    var cbuf = {};
    var lines = data.split("\n");
    var obj = JSON.parse(lines[0]);
    if (obj.annotations) {
        cbuf.annotations = obj.annotations;
        cbuf.header = JSON.parse(lines[1]);
        start = 2;
    } else {
        cbuf.header = obj;
    }
    cbuf.data = [];
    for (var i = start; i < lines.length; i++) {
        var line = lines[i];
        var inFields = line.split('\t');

        var fields = [];
        fields[0] = new Date((cbuf.header.time + (cbuf.header.seconds_per_row*(i-start)))*1000);
        for (var j = 0; j < inFields.length; j++) {
            fields[j+1] = parseFloat(inFields[j]);
        }
        cbuf.data.push(fields);
    }
    return cbuf;
}

function heka_load_cbuf(url, callback) {
    var req = new XMLHttpRequest();
    var caller = this;
    req.onreadystatechange = function () {
        if (req.readyState == 4) {
            if (req.status == 200 ||
                req.status == 0) {
                callback(heka_parse_cbuf(req.responseText));
            }
        }
    };
    req.open("GET", url, true);
    req.send(null);
}`
}

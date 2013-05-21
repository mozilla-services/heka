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
	"fmt"
	"github.com/mozilla-services/heka/message"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

type DashboardOutputConfig struct {
	// IP address of the Dashboard HTTP interface (defaults to all interfaces on
	// port 4352 (HEKA))
	Address string `toml:"address"`
	// Working directory where the Dashboard output is written to; it also
	// serves as the root for the HTTP fileserver.
	WorkingDirectory string `toml:"working_directory"`
}

func (self *DashboardOutput) ConfigStruct() interface{} {
	return &DashboardOutputConfig{
		Address:          ":4352",
		WorkingDirectory: "./dashboard",
	}
}

type DashboardOutput struct {
	workingDirectory string
	terminationFile  string
	server           *http.Server
}

func (self *DashboardOutput) Init(config interface{}) (err error) {
	conf := config.(*DashboardOutputConfig)
	self.workingDirectory, _ = filepath.Abs(conf.WorkingDirectory)
	self.terminationFile = "heka_sandbox_termination.tsv"
	if err = os.MkdirAll(self.workingDirectory, 0700); err != nil {
		return
	}
	// remove the HTML files so they will be regenerated
	if matches, err := filepath.Glob(path.Join(self.workingDirectory, "*.html")); err == nil {
		for _, fn := range matches {
			os.Remove(fn)
		}
	}
	os.Remove(path.Join(self.workingDirectory, self.terminationFile))
	overwriteFile(path.Join(self.workingDirectory, "heka_report.html"), getReportHtml())
	overwriteFile(path.Join(self.workingDirectory, "heka_sandbox_termination.html"), getSandboxTerminationHtml())

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
		plc  *PipelineCapture
		pack *PipelinePack
		msg  *message.Message
	)
	for ok {
		select {
		case plc, ok = <-inChan:
			if !ok {
				break
			}
			pack = plc.Pack
			msg = pack.Message
			switch msg.GetType() {
			case "heka.all-report":
				fn := path.Join(self.workingDirectory, "heka_report.json")
				overwriteFile(fn, msg.GetPayload())
			case "heka.sandbox-output":
				tmp, ok := msg.GetFieldValue("payload_type")
				if ok {
					if pt, ok := tmp.(string); ok && pt == "cbuf" {
						html := path.Join(self.workingDirectory, msg.GetLogger()+".html")
						_, err := os.Stat(html)
						if err != nil {
							overwriteFile(html, fmt.Sprintf(getCbufTemplate(), msg.GetLogger(), msg.GetLogger()))
						}
						fn := path.Join(self.workingDirectory, msg.GetLogger()+"."+pt)
						overwriteFile(fn, msg.GetPayload())
					}
				}
			case "heka.sandbox-terminated":
				fn := path.Join(self.workingDirectory, self.terminationFile)
				if file, err := os.OpenFile(fn, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644); err == nil {
					line := fmt.Sprintf("%d\t%s\t%v\n", msg.GetTimestamp()/1e9, msg.GetLogger(), msg.GetPayload())
					file.WriteString(line)
					file.Close()
				}
			}
			plc.Pack.Recycle()
		case <-ticker:
			go h.PipelineConfig().allReportsMsg()
		}
	}
	return
}

func overwriteFile(filename, s string) {
	if file, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC+os.O_CREATE, 0644); err == nil {
		file.WriteString(s)
		file.Close()
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
<body class="yui3-skin-sam">
    <div id="report"></div>
<script>
YUI().use("datatable-base", "datasource", "datasource-jsonschema", "datatable-datasource", "datatable-sort", function (Y) {
var dataSource = new Y.DataSource.IO({source:"heka_report.json"});
dataSource.plug({fn: Y.Plugin.DataSourceJSONSchema, cfg: {
        schema: {
            resultListLocator: 'reports',
            resultFields: [
                'Plugin',
                'InChanCapacity',
                'InChanLength',
                'MatchChanCapacity',
                'MatchChanLength',
                'Memory',
                'MaxMemory',
                'MaxInstructions',
                'MaxOutput'
            ]
        }}
    });

var table = new Y.DataTable({
    columns: [{key: 'Plugin', sortable:true, formatter: '<a href="{value}.html">{value}</a>', allowHTML: true},
              {key: 'InChanCapacity', sortable:true},
              {key: 'InChanLength', sortable:true},
              {key: 'MatchChanCapacity', sortable:true},
              {key: 'MatchChanLength', sortable:true},
              {label: 'Sandbox (memory and output in bytes)', children: [
                {key:'Memory', sortable:true},
                {key:'MaxMemory', sortable:true},
                {key:'MaxInstructions', sortable:true},
                {key:'MaxOutput', sortable:true}
              ]}],
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
    <script src="http://people.mozilla.org/~mtrinkala/heka/heka.js"  type="text/javascript">
    </script>
    <script type="text/javascript">

    function load_complete(cbuf) {
        var name = "graph";
        var plural = "";
        if ((cbuf.header.seconds_per_row * cbuf.header.rows) / 3600 > 1) {
            plural = "s";
        }
        document.getElementById('title').innerHTML = "%s" + " - "
            + cbuf.header.seconds_per_row + " second aggregation for the last "
            + String((cbuf.header.seconds_per_row * cbuf.header.rows) / 3600) + " hour" + plural;
        var labels = ['Date'];
        for (var i = 0; i < cbuf.header.columns; i++) {
            labels.push(cbuf.header.column_info[i].name + " (" + cbuf.header.column_info[i].type + ")");
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
    }
    </script>
</head>
<body onload="heka_load_cbuf('%s.cbuf', load_complete);">
<p id="title" style="text-align: center">
</p>
</body>
</html>`
}

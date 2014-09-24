
TextTemplateEncoder
===================

.. versionadded:: 0.8

The TextTemplateEncoder supports the mapping of message data on to external
template files using the template syntax provided by Go's `text/template
package <http://golang.org/pkg/text/template/>`_. The template will be
executed with the following variables available for use within the template:

* Uuid
* Timestamp
* Type
* Logger
* Severity
* Payload
* EnvVersion
* Pid
* Hostname
* Fields

All of the above except `Fields` will be rendered as a simple substitution
value in the template. `Fields` will expose the dynamic message fields as
key/value pairs, available for use within the template via either direct
dereferencing or the `with` idiom. For example, the following two snippets
will produce identical output containing the value of a field named `foo`:

.. code-block:: txt

	{{with .Fields}}foo: {{.foo}}{{end}}

.. code-block:: txt

	foo: {{.Fields.foo}}

Note that only the first value of the first instance of a given field name
will be available to the template.

Config:

- template_files ([]string, required):
	A list of template file paths to be loaded. Only the first file in the
	list will be rendered, but any snippets or functions defined in subsequent
	templates can be explicitly referred to and included by the first. Any
	relative file paths will be computed relative to Heka's specified
	`share_dir`, which is `/usr/share/heka` by default.

- reload_templates (bool, optional):
	Specifies whether or not the template files should be automatically
	reloaded if they are modified while Heka is running. If false, then they
	will only be loaded at startup time, and Heka will need to be restarted to
	pick up any subsequent changes. Defaults to `true`.

- reload_interval (int, optional):
	Specifies the interval between checks to see if the templates must be
	reloaded, in seconds. Defaults to 60. A setting of 0 will mean that a
	reload check will be performed for every message received; this is not
	recommended except for debugging, or cases with a very low message volume.
	If `reload_templates` is set to `false`, then this setting will have no
	impact.

- timestamp_format (string, optional):
	A string representing the format to be used when rendering the Timestamp
	value in the template, adhering to the format specification mechanism
	defined in Go's `time package <http://golang.org/pkg/time/#pkg-
	constants>`_. Defaults to "2006-01-02T15:04:05Z07:00".

Example:

.. code-block:: ini

	[TextTemplateEncoder]
	template_files = ["message_template.txt"]

	[LogOutput]
	message_matcher = "TRUE"
	encoder = "TextTemplateEncoder"

Sample Template:

.. code-block:: txt

	=Test message template=
	UUID: {{.Uuid}}
	Timestamp: {{.Timestamp}}
	Type: {{.Type}}
	Logger: {{.Logger}}
	Severity: {{.Severity}}
	Payload: {{.Payload}}
	EnvVersion: {{.EnvVersion}}
	Pid: {{.Pid}}
	Hostname: {{.Hostname}}
	foo: {{.Fields.foo}}
	{{with .Fields}}snarf: {{.snarf}}{{end}}

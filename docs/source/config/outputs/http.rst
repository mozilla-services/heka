
.. versionadded:: 0.6

HttpOutput
==========

A simple output plugin that uses HTTP GET, POST, or PUT requests to deliver
data to an HTTP endpoint. The output supports batching, i.e. accumulating data
from multiple messages and combining it into a single HTTP request. When
batching is used (specified by use of the `flush_interval` and/or
`flush_count` configuration settings) each message will be encoded separately,
and the output of the various encoding operations will be concatenated.

When using POST or PUT request methods the encoded output will be uploaded as
the request body. When using GET the encoded output will be ignored.

Config:

- address (string):
	URL address of HTTP server to which requests should be sent. Must begin
	with "http://" or "https://".
- method (string, optional):
	HTTP request method to use, must be one of GET, POST, or PUT. Defaults to
	POST.
- flush_interval (uint32, optional):
	Elapsed interval, in milliseconds, after which an HTTP request will be
	made. A value of zero means time elapsed will never trigger a flush.
	Defaults to zero.
- flush_count (uint32, optional):
	Number of messages that must be processed before an HTTP request will be
	made. A value of zero means that message count will never trigger a flush.
	Defaults to one. At least one of `flush_interval` and `flush_count` must
	be non-zero.
- username (string, optional):
	If specified, HTTP Basic Auth will be used with the provided user name.
- password (string, optional):
	If specified, HTTP Basic Auth will be used with the provided password.
- headers (subsection, optional):
    It is possible to inject arbitrary HTTP headers into each outgoing request
    by adding a TOML subsection entitled "headers" to you HttpOutput config
    section. All entries in the subsection must be a list of string values.
- tls (subsection, optional):
	A sub-section that specifies the settings to be used for any SSL/TLS
	encryption. This will only have any impact if an "https://" address is
	used. See :ref:`tls`.

Example:

.. code-block:: ini

	[PayloadEncoder]

	[influxdb]
	message_matcher = "Type == 'influx.formatted'"
	address = "http://influxdb.example.com:8086/db/stats/series"
	encoder = "PayloadEncoder"
	username = "MyUserName"
	password = "MyPassword"

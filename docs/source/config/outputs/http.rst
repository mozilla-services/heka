.. _config_http_output:

.. versionadded:: 0.6

HTTP Output
===========

Plugin Name: **HttpOutput**

A very simple output plugin that uses HTTP GET, POST, or PUT requests to
deliver data to an HTTP endpoint. When using POST or PUT request methods the
encoded output will be uploaded as the request body. When using GET the
encoded output will be ignored.

This output doesn't support any request batching; each received message will
generate an HTTP request. Batching can be achieved by use of a filter plugin
that accumulates message data, periodically emitting a single message
containing the batched, encoded HTTP request data in the payload. An
HttpOutput can then be configured to capture these batch messages, using a
:ref:`config_payloadencoder` to extract the message payload.

For now the HttpOutput only supports statically defined request parameters
(URL, headers, auth, etc.). Future iterations will provide a mechanism for
dynamically specifying these values on a per-message basis.

Config:

- address (string):
	URL address of HTTP server to which requests should be sent. Must begin
	with "http://" or "https://".
- method (string, optional):
	HTTP request method to use, must be one of GET, POST, or PUT. Defaults to
	POST.
- username (string, optional):
	If specified, HTTP Basic Auth will be used with the provided user name.
- password (string, optional):
	If specified, HTTP Basic Auth will be used with the provided password.
- headers (subsection, optional):
    It is possible to inject arbitrary HTTP headers into each outgoing request
    by adding a TOML subsection entitled "headers" to you HttpOutput config
    section. All entries in the subsection must be a list of string values.
- http_timeout(uint, optional):
    Time in milliseconds to wait for a response for each http request. This
    may drop data as there is currently no retry. Default is 0 (no timeout)
- tls (subsection, optional):
	A sub-section that specifies the settings to be used for any SSL/TLS
	encryption. This will only have any impact if an "https://" address is
	used. See :ref:`tls`.

Example:

.. code-block:: ini

	[PayloadEncoder]

	[influxdb]
	type = "HttpOutput"
	message_matcher = "Type == 'influx.formatted'"
	address = "http://influxdb.example.com:8086/db/stats/series"
	encoder = "PayloadEncoder"
	username = "MyUserName"
	password = "MyPassword"


ESLogstashV0Encoder
=================

This encoder serializes a Heka message into a JSON format, preceded by a
separate JSON structure containing information required for ElasticSearch
`BulkAPI
<http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/docs-
bulk.html>`_ indexing. The message JSON structure uses the original (i.e.
"v0") schema popularized by `Logstash <http://logstash.net/>`_. Using this
schema can aid integration with existing Logstash deployments. This schema
also plays nicely with the default Logstash dashboard provided by `Kibana
<http://www.elasticsearch.org/overview/kibana/>`_.

The JSON serialization is done by hand, without using Go's stdlib JSON
marshalling. This is so serialization can succeed even if the message contains
invalid UTF-8 characters, which will be encoded as U+FFFD.

.. _eslogstashv0encoder_settings

Config:

- index (string):
    Name of the ES index into which the messages will be inserted. Supports
    interpolation of message field values (from 'Type', 'Hostname', 'Pid',
    'UUID', 'Logger', 'EnvVersion', 'Severity', a field name, or a timestamp
    format) with the use of '%{}' chars, so '%{Hostname}-%{Logger}-data' would
    add the records to an ES index called 'some.example.com-processname-data'.
    Defaults to 'logstash-%{2006.01.02}'.
- type_name (string):
    Name of ES record type to create. Supports interpolation of message field
    values (from 'Type', 'Hostname', 'Pid', 'UUID', 'Logger', 'EnvVersion',
    'Severity', field name, or a timestamp format) with the use of '%{}'
    chars, so '%{Hostname}-stat' would create an ES record with a type of
    'some.example.com-stat'. Defaults to 'message'.
- fields ([]string):
    The 'fields' parameter specifies that only specific message data should be
    indexed into ElasticSearch. Available fields to choose are "Uuid",
    "Timestamp", "Type", "Logger", "Severity", "Payload", "EnvVersion", "Pid",
    "Hostname", and "Fields" (where "Fields" causes the inclusion of any and
    all dynamically specified message fields. Defaults to including all of the
    supported message fields.
- es_index_from_timestamp (bool):
    When generating the index name use the timestamp from the message instead
    of the current time. Defaults to false.
- id (string):
    Allows you to optionally specify the document id for ES to use. Useful for
    overwriting existing ES documents. If the value specified is placed within
    %{}, it will be interpolated to its Field value. Default is allow ES to
    auto-generate the id.
- raw_bytes_fields ([]string):
    This specifies a set of fields which will be passed through to the encoded
    JSON output without any processing or escaping. This is useful for fields
    which contain embedded JSON objects to prevent the embedded JSON from
    being escaped as normal strings. Only supports dynamically specified
    message fields.

Example

.. code-block:: ini

	[ESLogstashV0Encoder]
	es_index_from_timestamp = true
	type_name = "%{Type}"

	[ElasticSearchOutput]
	message_matcher = "Type == 'nginx.access'"
	encoder = "ESLogstashV0Encoder"
	flush_interval = 50
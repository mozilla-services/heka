
ElasticSearchOutput
===================

Output plugin that serializes messages into JSON structures and uses HTTP requests
to insert them into an ElasticSearch database.

Config:

- cluster (string):
    ElasticSearch cluster name. Defaults to "elasticsearch"
- index (string):
    Name of the ES index into which the messages will be inserted. Supports
    interpolation of message field values (from 'Type', 'Hostname', 'Pid',
    'UUID', 'Logger', 'EnvVersion', 'Severity', a field name, or a timestamp
    format) with the use of '%{}' chars, so '%{Hostname}-%{Logger}-data' would
    add the records to an ES index called 'some.example.com-processname-data'.
    Defaults to 'heka-%{2006.01.02}'.
- type_name (string):
    Name of ES record type to create. Supports interpolation of message field
    values (from 'Type', 'Hostname', 'Pid', 'UUID', 'Logger', 'EnvVersion',
    'Severity', field name, or a timestamp format) with the use of '%{}'
    chars, so '%{Hostname}-stat' would create an ES record with a type of
    'some.example.com-stat'. Defaults to 'message'.
- flush_interval (int):
    Interval at which accumulated messages should be bulk indexed into
    ElasticSearch, in milliseconds. Defaults to 1000 (i.e. one second).
- flush_count (int):
    Number of messages that, if processed, will trigger them to be bulk
    indexed into ElasticSearch. Defaults to 10.
- fields ([]string):
    If the format is "clean", then the 'fields' parameter can be used to
    specify that only specific message data should be indexed into
    ElasticSearch. Available fields to choose are "Uuid", "Timestamp", "Type",
    "Logger", "Severity", "Payload", "EnvVersion", "Pid", "Hostname", and
    "Fields" (where "Fields" causes the inclusion of any and all dynamically
    specified message fields. Defaults to all.
- timestamp (string):
    Format to use for timestamps in generated ES documents. Defaults to
    "2006-01-02T15:04:05.000Z".
- server (string):
    ElasticSearch server URL. Supports http://, https:// and udp:// urls.
    Defaults to "http://localhost:9200".
- ESIndexFromTimestamp (bool):
    When generating the index name use the timestamp from the message
    instead of the current time. Defaults to false.
- id (string):
    .. versionadded:: 0.5

    Allows you to optionally specify the document id for ES to use. Useful for
    overwriting existing ES documents. If the value specified is placed within
    %{}, it will be interpolated to its Field value. Default is allow ES to
    auto-generate the id.
- http_timeout (int):
    Time in milliseconds to wait for a response for each http post to ES.
    This may drop data as there is currently no retry.
    Default is 0 (infinite)

.. deprecated:: 0.6
    Use encoder instead.

- format (string):
    Message serialization format, either "clean", "logstash_v0", "payload" or
    "raw". "clean" is a more concise JSON representation of the message,
    "logstash_v0" outputs in a format similar to Logstash's original (i.e.
    "version 0") ElasticSearch schema, "payload" passes the message payload
    directly into ElasticSearch, and "raw" is a full JSON representation of
    the message. Defaults to "clean".

- raw_bytes_field ([]string):
    This option allows you to specify a list of fields to be passed through
    the "clean" or "logstash_v0" formatters unchanged.

Example:

.. code-block:: ini

    [ElasticSearchOutput]
    message_matcher = "Type == 'sync.log'"
    cluster = "elasticsearch-cluster"
    index = "synclog-%{field1}-%{2006.01.02.15.04.05}"
    type_name = "sync.log.line-%{field1}"
    server = "http://es-server:9200"
    flush_interval = 5000
    flush_count = 10
    id = %{id}
    encoder = "PayloadEncoder"

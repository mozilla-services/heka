
ElasticSearchOutput
===================

Output plugin that uses HTTP or UDP to insert records into an ElasticSearch
database. Note that it is up to the specified encoder to both serialize the
message into a JSON structure *and* to prepend that with the appropriate
ElasticSearch `BulkAPI
<http://www.elasticsearch.org/guide/en/elasticsearch/reference/current/docs-
bulk.html>`_ indexing JSON. Usually this output is used in conjunction with an
ElasticSearch-specific encoder plugin, such as :ref:`config_esjsonencoder`,
:ref:`config_eslogstashv0encoder`, or :ref:`config_espayload`.

Config:

- flush_interval (int):
    Interval at which accumulated messages should be bulk indexed into
    ElasticSearch, in milliseconds. Defaults to 1000 (i.e. one second).
- flush_count (int):
    Number of messages that, if processed, will trigger them to be bulk
    indexed into ElasticSearch. Defaults to 10.
- server (string):
    ElasticSearch server URL. Supports http://, https:// and udp:// urls.
    Defaults to "http://localhost:9200".
- http_timeout (int):
    Time in milliseconds to wait for a response for each http post to ES. This
    may drop data as there is currently no retry. Default is 0 (no timeout).

Example:

.. code-block:: ini

    [ElasticSearchOutput]
    message_matcher = "Type == 'sync.log'"
    server = "http://es-server:9200"
    flush_interval = 5000
    flush_count = 10
    encoder = "ESJsonEncoder"

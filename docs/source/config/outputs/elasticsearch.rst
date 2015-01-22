
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
- connect_timeout (int):
    Time in milliseconds to wait for a server name resolving and connection to ES.
    It's included in an overall time (see 'http_timeout' option), if they both are set.
    Default is 0 (no timeout).
- http_timeout (int):
    Time in milliseconds to wait for a response for each http post to ES. This
    may drop data as there is currently no retry. Default is 0 (no timeout).
- http_disable_keepalives (bool):
    Specifies whether or not re-using of established TCP connections to
    ElasticSearch should be disabled. Defaults to false, that means using
    both HTTP keep-alive mode and TCP keep-alives. Set it to true to close
    each TCP connection after 'flushing' messages to ElasticSearch.
- username (string):
    The username to use for HTTP authentication against the ElasticSearch host.
    Defaults to "" (i. e. no authentication).
- password (string):
    The password to use for HTTP authentication against the ElasticSearch host.
    Defaults to "" (i. e. no authentication).

.. versionadded:: 0.9

- use_buffering: (bool, optional):
    Buffer records to a disk-backed buffer on the Heka server before writing them to ElasticSearch.
    Defaults to true.
- queue_max_buffer_size (uint64, optional):
    Defines maximum queue buffer size, in bytes. Defaults to 0, which means no
    max.
- queue_full_action (string, optional):
    Specifies how Heka should behave when the queue reaches the specified
    maximum capacity. There are currently three possible actions:

        - `shutdown` - Shuts down Heka.
        - `drop` - New messages are dropped until queue is available again.
          Already queued messages are unaffected.
        - `block` - Blocks processing of messages, tries to push last message
          until its possible.

    Defaults to `shutdown`.

Example:

.. code-block:: ini

    [ElasticSearchOutput]
    message_matcher = "Type == 'sync.log'"
    server = "http://es-server:9200"
    flush_interval = 5000
    flush_count = 10
    encoder = "ESJsonEncoder"


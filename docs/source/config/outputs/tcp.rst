.. _config_tcp_output:

TCP Output
==========

Plugin Name: **TcpOutput**

Output plugin that delivers Heka message data to a listening TCP connection.
Can be used to deliver messages from a local running Heka agent to a remote
Heka instance set up as an aggregator and/or router, or to any other arbitrary
listening TCP server that knows how to process the encoded data.

Config:

- address (string):
    An IP address:port to which we will send our output data.
- use_tls (bool, optional):
    Specifies whether or not SSL/TLS encryption should be used for the TCP
    connections. Defaults to false.

.. versionadded:: 0.5

- tls (TlsConfig, optional):
    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.
- ticker_interval (uint, optional):
    Specifies how often, in seconds, the output queue files are rolled.
    Defaults to 300.

.. versionadded:: 0.6

- local_address (string, optional):
    A local IP address to use as the source address for outgoing  traffic to
    this destination. Cannot currently be combined with TLS connections.
- encoder (string, optional):
    Specifies which of the registered encoders should be used for converting
    Heka messages to binary data that is sent out over the TCP connection.
    Defaults to the always available "ProtobufEncoder".
- use_framing (bool, optional):
    Specifies whether or not the encoded data sent out over the TCP connection
    should be delimited by Heka's :ref:`stream_framing`. Defaults to true if a
    ProtobufEncoder is used, false otherwise.
- keep_alive (bool):
    Specifies whether or not `TCP keepalive
    <http://en.wikipedia.org/wiki/Keepalive#TCP_keepalive>`_ should be used
    for established TCP connections. Defaults to false.
- keep_alive_period (int):
    Time duration in seconds that a TCP connection will be maintained before
    keepalive probes start being sent. Defaults to 7200 (i.e. 2 hours).

.. versionadded:: 0.9

- queue_max_buffer_size (uint64):
    Defines maximum queue buffer size, in bytes. Defaults to 0, which means no
    max.
- queue_full_action (string, optional):
    Specifies how Heka should behave when the queue reaches the specified
    maximum capacity. There are currently three possible actions:

      - `shutdown`: Shutdowns heka.
      - `drop`: Messages are dropped until queue is available again. Already queued
                messages are unaffected.
      - `block`: Blocks processing of messages, tries to push last message
                 until its possible.

    Defaults to `shutdown`.

Example:

.. code-block:: ini

    [aggregator_output]
    type = "TcpOutput"
    address = "heka-aggregator.mydomain.com:55"
    local_address = "127.0.0.1"
    message_matcher = "Type != 'logfile' && Type != 'heka.counter-output' && Type != 'heka.all-report'"

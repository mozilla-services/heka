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
    encryption. This will only have any impact if ``use_tls`` is set to true.
    See :ref:`tls`.

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

.. versionadded:: 0.10

- use_buffering (bool, optional):
    Buffer records to a disk-backed buffer on the Heka server before sending
    them out over the TCP connection. Defaults to true.
- buffering (QueueBufferConfig, optional):
    All of the :ref:`buffering <buffering>` config options are set to the
    standard default options, except for `cursor_update_count`, which is set to
    50 instead of the standard default of 1.
- reconnect_after (int, optional):
    Re-establish the TCP connection after the specified number of successfully
    delivered messages.  Defaults to 0 (no reconnection).

Example:

.. code-block:: ini

    [aggregator_output]
    type = "TcpOutput"
    address = "heka-aggregator.mydomain.com:55"
    local_address = "127.0.0.1"
    message_matcher = "Type != 'logfile' && Type !~ /^heka\./'"

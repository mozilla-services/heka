.. _config_tcp_input:

TCP Input
=========

Plugin Name: **TcpInput**

Listens on a specific TCP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name
is added to the pipeline pack and can be use to accept messages using the
message_signer configuration option.

Config:

- address (string):
    An IP address:port on which this plugin will listen.

.. versionadded:: 0.4

- decoder (string):
    Defaults to "ProtobufDecoder".

.. versionadded:: 0.5

- use_tls (bool):
    Specifies whether or not SSL/TLS encryption should be used for the TCP
    connections. Defaults to false.
- tls (TlsConfig):
    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.
- net (string, optional, default: "tcp")
    Network value must be one of: "tcp", "tcp4", "tcp6", "unix" or "unixpacket".

.. versionadded:: 0.6

- keep_alive (bool):
    Specifies whether or not `TCP keepalive
    <http://en.wikipedia.org/wiki/Keepalive#TCP_keepalive>`_ should be used
    for established TCP connections. Defaults to false.
- keep_alive_period (int):
    Time duration in seconds that a TCP connection will be maintained before
    keepalive probes start being sent. Defaults to 7200 (i.e. 2 hours).

.. versionadded:: 0.9

- splitter (string):
    Defaults to "HekaFramingSplitter".

Example:

.. code-block:: ini

    [TcpInput]
    address = ":5565"

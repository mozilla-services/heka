
TcpOutput
=========

Output plugin that serializes messages into the Heka protocol format and
delivers them to a listening TCP connection. Can be used to deliver messages
from a local running Heka agent to a remote Heka instance set up as an
aggregator and/or router.

Config:

- address (string):
    An IP address:port to which we will send our output data.
- use_tls (bool):
    Specifies whether or not SSL/TLS encryption should be used for the TCP
    connections. Defaults to false.

.. versionadded:: 0.5

- tls (TlsConfig):
    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.
- ticker_interval (uint):
    Specifies how often, in seconds, the output queue files are rolled.
    Defaults to 300.

.. versionadded:: 0.6

- local_address (string):
    An optional local IP address to use as the source address for outgoing 
    traffic to this destination. Cannot currently be combined with TLS connections.

Example:

.. code-block:: ini

    [aggregator_output]
    type = "TcpOutput"
    address = "heka-aggregator.mydomain.com:55"
    local_address = "127.0.0.1"
    message_matcher = "Type != 'logfile' && Type != 'heka.counter-output' && Type != 'heka.all-report'"

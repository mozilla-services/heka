.. _config_amqp_output:

AMQP Output
===========

Plugin Name: **AMQPOutput**

Connects to a remote AMQP broker (RabbitMQ) and sends messages to the
specified queue. The message is serialized if specified, otherwise only
the raw payload of the message will be sent. As AMQP is dynamically
programmable, the broker topology needs to be specified.

Config:

- url (string):
    An AMQP connection string formatted per the `RabbitMQ URI Spec
    <http://www.rabbitmq.com/uri-spec.html>`_.
- exchange (string):
    AMQP exchange name
- exchange_type (string):
    AMQP exchange type (`fanout`, `direct`, `topic`, or `headers`).
- exchange_durability (bool):
    Whether the exchange should be configured as a durable exchange. Defaults
    to non-durable.
- exchange_auto_delete (bool):
    Whether the exchange is deleted when all queues have finished and there
    is no publishing. Defaults to auto-delete.
- routing_key (string):
    The message routing key used to bind the queue to the exchange. Defaults
    to empty string.
- persistent (bool):
    Whether published messages should be marked as persistent or transient.
    Defaults to non-persistent.
- retries (RetryOptions, optional):
    A sub-section that specifies the settings to be used for restart behavior.
    See :ref:`configuring_restarting`

.. versionadded:: 0.6

- content_type (string):
     MIME content type of the payload used in the AMQP header. Defaults to
     "application/hekad".
- encoder (string, optional)
    Specifies which of the registered encoders should be used for converting
    Heka messages to binary data that is sent out over the AMQP connection.
    Defaults to the always available "ProtobufEncoder".
- use_framing (bool, optional):
    Specifies whether or not the encoded data sent out over the TCP connection
    should be delimited by Heka's :ref:`stream_framing`. Defaults to true.

.. versionadded:: 0.6

- tls (TlsConfig):
    An optional sub-section that specifies the settings to be used for any
    SSL/TLS encryption. This will only have any impact if `URL` uses the
    `AMQPS` URI scheme. See :ref:`tls`.

Example (that sends log lines from the logger):

.. code-block:: ini

    [AMQPOutput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchange_type = "fanout"
    message_matcher = 'Logger == "TestWebserver"'

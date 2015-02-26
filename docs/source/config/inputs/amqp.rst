.. _config_amqp_input:

AMQP Input
==========

Plugin Name: **AMQPInput**

Connects to a remote AMQP broker (RabbitMQ) and retrieves messages from the
specified queue. As AMQP is dynamically programmable, the broker topology
needs to be specified in the plugin configuration.

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
- prefetch_count (int):
    How many messages to fetch at once before message acks are sent. See
    `RabbitMQ performance measurements
    <http://www.rabbitmq.com/blog/2012/04/25/rabbitmq-performance-
    measurements-part-2/>`_ for help in tuning this number. Defaults to 2.
- queue (string):
    Name of the queue to consume from, an empty string will have the broker
    generate a name for the queue. Defaults to empty string.
- queue_durability (bool):
    Whether the queue is durable or not. Defaults to non-durable.
- queue_exclusive (bool):
    Whether the queue is exclusive (only one consumer allowed) or not.
    Defaults to non-exclusive.
- queue_auto_delete (bool):
    Whether the queue is deleted when the last consumer un-subscribes.
    Defaults to auto-delete.
- queue_ttl (int):
    Allows ability to specify TTL in milliseconds on Queue declaration for
    expiring messages. Defaults to undefined/infinite.
- retries (RetryOptions, optional):
    A sub-section that specifies the settings to be used for restart behavior.
    See :ref:`configuring_restarting`

.. versionadded:: 0.6

- tls (TlsConfig):
    An optional sub-section that specifies the settings to be used for any
    SSL/TLS encryption. This will only have any impact if `URL` uses the
    `AMQPS` URI scheme. See :ref:`tls`.

.. versionadded:: 0.9

- read_only (bool):
    Whether the AMQP user is read-only. If this is true the exchange, queue
    and binding must be declared before starting Heka. Defaults to false.

Since many of these parameters have sane defaults, a minimal configuration to
consume serialized messages would look like:

.. code-block:: ini

    [AMQPInput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchange_type = "fanout"

Or you might use a PayloadRegexDecoder to parse OSX syslog messages with the
following:

.. code-block:: ini

    [AMQPInput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchange_type = "fanout"
    decoder = "logparser"

    [logparser]
    type = "MultiDecoder"
    subs = ["logline", "leftovers"]

    [logline]
    type = "PayloadRegexDecoder"
    MatchRegex = '\w+ \d+ \d+:\d+:\d+ \S+ (?P<Reporter>[^\[]+)\[(?P<Pid>\d+)](?P<Sandbox>[^:]+)?: (?P Remaining>.*)'

        [logline.MessageFields]
        Type = "amqplogline"
        Hostname = "myhost"
        Reporter = "%Reporter%"
        Remaining = "%Remaining%"
        Logger = "%Logger%"
        Payload = "%Remaining%"

    [leftovers]
    type = "PayloadRegexDecoder"
    MatchRegex = '.*'

        [leftovers.MessageFields]
        Type = "drop"
        Payload = ""

.. _config_kafka_output:

Kafka Output
============

Plugin Name: **KafkaOutput**

Connects to a Kafka broker and sends messages to the specified topic.

Config:

- id (string)
    Client ID string. Default is the hostname.
- addrs ([]string)
    List of brokers addresses.
- metadata_retries (int)
    How many times to retry a metadata request when a partition is in the middle
    of leader election. Default is 3.
- wait_for_election (uint32)
    How long to wait for leader election to finish between retries (in
    milliseconds). Default is 250.
- background_refresh_frequency (uint32)
    How frequently the client will refresh the cluster metadata in the
    background (in milliseconds). Default is 600000 (10 minutes). Set to 0 to
    disable.

- max_open_reqests (int)
    How many outstanding requests the broker is allowed to have before blocking
    attempts to send. Default is 4.
- dial_timeout (uint32)
    How long to wait for the initial connection to succeed before timing out and
    returning an error (in milliseconds).  Default is 60000 (1 minute).
- read_timeout (uint32)
    How long to wait for a response before timing out and returning an error (in
    milliseconds).  Default is 60000 (1 minute).
- write_timeout (uint32)
     How long to wait for a transmit to succeed before timing out and returning
     an error (in milliseconds).  Default is 60000 (1 minute).

- partitioner (string)
    Chooses the partition to send messages to. The valid values are *Random*,
    *RoundRobin*, *Hash*. Default is Random.
- hash_variable (string)
    The message variable used for the Hash partitioner only. The variables are
    restricted to *Type*, *Logger*, *Hostname*, *Payload* or any of the
    message's dynamic field values. All dynamic field values will be converted
    to a string representation. Field specifications are the same as with the
    :ref:`message_matcher` e.g. Fields[foo][0][0].
- topic_variable (string)
    The message variable used as the Kafka topic (cannot be used in conjunction
    with the 'topic' configuration). The variable restrictions are the same as
    the hash_variable.
- topic (string)
    A static Kafka topic (cannot be used in conjunction with the
    'topic_variable' configuration).

- required_acks (string)
    The level of acknowledgement reliability needed from the broker. The valid
    values are *NoResponse*, *WaitForLocal*, *WaitForAll*. Default is
    WaitForLocal.
- timeout (uint32)
    The maximum duration the broker will wait for the receipt of the number of
    RequiredAcks (in milliseconds). This is only relevant when RequiredAcks is
    set to WaitForAll. Default is no timeout.
- compression_codec (string)
    The type of compression to use on messages.  The valid values are *None*,
    *GZIP*, *Snappy*. Default is None.
- max_buffer_time (uint32)
    The maximum duration to buffer messages before triggering a flush to the
    broker (in milliseconds). Default is 1.
- max_buffered_bytes (uint32)
    The threshold number of bytes buffered before triggering a flush to the
    broker. Default is 1.
- back_pressure_threshold_bytes (uint32)
    The maximum number of bytes allowed to accumulate in the buffer before
    back-pressure is applied to QueueMessage. Without this, queueing messages
    too fast will cause the producer to construct requests larger than the
    MaxRequestSize (100 MiB). Default is 50 * 1024 * 1024 (50 MiB), cannot be
    more than (MaxRequestSize - 10 KiB).

Example (send various Fxa messages to a static Fxa topic):

.. code-block:: ini

    [FxaKafkaOutput]
    type = "KafkaOutput"
    message_matcher = "Logger == 'FxaAuthWebserver' || Logger == 'FxaAuthServer'"
    topic = "Fxa"
    addrs = ["localhost:9092"]
    encoder = "ProtobufEncoder"

.. _config_kafka_input:

Kafka Input
===========

Plugin Name: **KafkaInput**

Connects to a Kafka broker and subscribes to messages from the specified topic
and partition.

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

- topic (string)
    Kafka topic (must be set).
- partition (int32)
    Kafka topic partition. Default is 0.
- group (string)
    A string that uniquely identifies the group of consumer processes to which
    this consumer belongs. By setting the same group id multiple processes
    indicate that they are all part of the same consumer group. Default is the
    *id*.

- default_fetch_size (int32)
    The default (maximum) amount of data to fetch from the broker in each
    request. The default is 32768 bytes.
- min_fetch_size (int32)
    The minimum amount of data to fetch in a request - the broker will wait
    until at least this many bytes are available. The default is 1, as 0 causes
    the consumer to spin when no messages are available.
- max_message_size (int32)
    The maximum permittable message size - messages larger than this will return
    MessageTooLarge. The default of 0 is treated as no limit.
- max_wait_time (uint32)
    The maximum amount of time the broker will wait for min_fetch_size bytes to
    become available before it returns fewer than that anyways. The default is
    250ms, since 0 causes the consumer to spin when no events are available.
    100-500ms is a reasonable range for most cases.
- offset_method (string)
    The method used to determine at which offset to begin consuming messages.
    The valid values are:

    - *Manual* Heka will track the offset and resume from where it last left off (default).
    - *Newest* Heka will start reading from the most recent available offset.
    - *Oldest* Heka will start reading from the oldest available offset.

- event_buffer_size (int)
    The number of events to buffer in the Events channel. Having this non-zero
    permits the consumer to continue fetching messages in the background while
    client code consumes events, greatly improving throughput. The default is
    16.

Example 1: Read Fxa messages from partition 0.

.. code-block:: ini

    [FxaKafkaInputTest]
    type = "KafkaInput"
    topic = "Fxa"
    addrs = ["localhost:9092"]

Example 2: Send messages between two Heka instances via a Kafka broker.

.. code-block:: ini

    # On the producing instance
    [KafkaOutputExample]
    type = "KafkaOutput"
    message_matcher = "TRUE"
    topic = "heka"
    addrs = ["kafka-broker:9092"]
    encoder = "ProtobufEncoder"

.. code-block:: ini

    # On the consuming instance
    [KafkaInputExample]
    type = "KafkaInput"
    topic = "heka"
    addrs = ["kafka-broker:9092"]
    splitter = "KafkaSplitter"
    decoder = "ProtobufDecoder"

    [KafkaSplitter]
    type = "NullSplitter"
    use_message_bytes = true


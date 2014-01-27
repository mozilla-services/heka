.. testing:

============
Testing Heka
============

Flood
=====
Flood is a Heka load test tool; it is capable of generating a large number of
messages to exercise Heka using different protocols, message types, and error
conditions.

Command Line Options
--------------------
heka-flood [``-config`` `config_file`] [``-test`` `config_section_name`]

Configuration Variables
-----------------------
- test (object): Name of the test section (toml key) in the configuration file.
- ip_address (string): IP address of the Heka server.
- sender (string): tcp or udp
- pprof_file (string): The name of the file to save the profiling data to.
- encoder (string): protobuf or json
- num_messages (int): The number of messages to be sent, 0 for infinite.
- corrupt_percentage (float): The percentage of messages that will be randomly corrupted.
- signed_percentage (float): The percentage of message that will signed.
- variable_size_messages (bool): True, if a random selection of variable size messages are to be sent.  False, if a single fixed message will be sent.
- signer (object): Signer information for the encoder.
    - name (string): The name of the signer.
    - hmac_hash (string): md5 or sha1
    - hmac_key (string): The key the message will be signed with.
    - version (int): The version number of the hmac_key.
- ascii_only (bool): True, if generated message payloads should only contain ASCII characters. False, if message payloads should contain arbitrary binary data. Defaults to false.
- use_tls (bool): Specifies whether or not SSL/TLS encryption should be used for the TCP connections. Defaults to false.
- tls (TlsConfig): A sub-section that specifies the settings to be used for any SSL/TLS encryption. This will only have any impact if `use_tls` is set to true. See :ref:`tls`.

Example

.. code-block:: ini

    [default]                                  
    ip_address          = "127.0.0.1:5565"
    sender              = "tcp"
    pprof_file          = ""
    encoder             = "protobuf"
    num_messages        = 0
    corrupt_percentage  = 0.0001
    signed_percentage   = 0.00011
    variable_size_messages = true
    [default.signer]
        name            = "test"
        hmac_hash       = "md5"
        hmac_key        = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
        version          = 0


Inject
======
Inject is a Heka client allowing for the injecting of arbitrary messages into the Heka pipeline. It is capable of generating a message of specified message variables with values. It allows for quickly testing plugins. Inject requires TcpInput with Protobufs encoder availability.

Command Line Options
--------------------
heka-inject [``-heka`` `Heka instance to connect`] [``-hostname`` `message hostname`] [``-logger`` `message logger`] [``-payload`` `message payload`] [``-pid`` `message pid`] [``-severity`` `message severity`] [``-type`` `message type`]


Example

heka-inject -payload="Test message to for high severity." -severity=1

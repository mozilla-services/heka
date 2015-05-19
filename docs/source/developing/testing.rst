.. testing:

============
Testing Heka
============

heka-flood
==========
heka-flood is a Heka load test tool; it is capable of generating a large
number of messages to exercise Heka using different protocols, message types,
and error conditions.

Command Line Options
--------------------
- -config="flood.toml": Path to heka-flood config file
- -test="default": Name of config file defined test to run

Example::

    heka-flood -config="/etc/flood.toml" -test="my_test_name"

Configuration Variables
-----------------------

- test (object):
    Name of the test section (toml key) in the configuration file.

- ip_address (string):
    IP address of the Heka server.

- sender (string):
    tcp or udp

- pprof_file (string):
    The name of the file to save the profiling data to.

- encoder (string):
    protobuf or json

- num_messages (int):
    The number of messages to be sent, 0 for infinite.

- message_interval (string):
    Duration of time to delay between the sending of each message. Accepts
    duration values as supported by Go's `time.ParseDuration function
    <http://golang.org/pkg/time/#ParseDuration>`_. Default of 0 means no
    delay.

- corrupt_percentage (float):
    The percentage of messages that will be randomly corrupted.

- signed_percentage (float):
    The percentage of message that will signed.

- variable_size_messages (bool):
    True, if a random selection of variable size messages are to be sent.
    False, if a single fixed message will be sent.

- signer (object): Signer information for the encoder.

    - name (string): The name of the signer.
    - hmac_hash (string): md5 or sha1
    - hmac_key (string): The key the message will be signed with.
    - version (int): The version number of the hmac_key.

- ascii_only (bool):
    True, if generated message payloads should only contain ASCII characters.
    False, if message payloads should contain arbitrary binary data. Defaults
    to false.

.. versionadded:: 0.5

- use_tls (bool):
    Specifies whether or not SSL/TLS encryption should be used for the TCP
    connections. Defaults to false.

- tls (TlsConfig):
    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.

.. versionadded:: 0.9

- max_message_size (uint32):
    The maximum size of the message that will be sent by heka-flood.

.. versionadded:: 0.10

- reconnect_on_error (bool):
    Defines if `heka-flood` should try to reconnect with backend after connection error. Exits if reconnect_on_error is set to false.
    Defaults to false.

- reconnect_interval (int):
    Specifies interval (in seconds) after which `heka-flood` will try to recreate connection with backend.
    Defaults to 5s.

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

heka-inject
===========
.. versionadded:: 0.5

heka-inject is a Heka client allowing for the injecting of arbitrary messages
into the Heka pipeline. It is capable of generating a message of specified
message variables with values. It allows for quickly testing plugins. Inject
requires TcpInput with Protobufs encoder availability.

Command Line Options
--------------------
- -heka: Heka instance to connect
- -hostname: message hostname
- -logger: message logger
- -payload: message payload
- -pid: message pid
- -severity: message severity
- -type: message type

Example::

    heka-inject -payload="Test message with high severity." -severity=1

heka-cat
========
.. versionadded:: 0.5

A command-line utility for counting, viewing, filtering, and extracting Heka
protobuf logs.

Command Line Options
--------------------
- -format="txt": output format [txt|json|heka|count]
- -match="TRUE": message_matcher filter expression
- -offset=0: starting offset for the input file in bytes
- -output="": output filename, defaults to stdout
- -tail=false: don't exit on EOF
- `input filename`

Example::

    heka-cat -format=count -match="Fields[status] == 404" test.log

Output::

    Input:test.log  Offset:0  Match:Fields[status] == 404  Format:count  Tail:false  Output:
    Processed: 1002646, matched: 15660 messages
    

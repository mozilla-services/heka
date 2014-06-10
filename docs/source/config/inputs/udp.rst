
UdpInput
========

Listens on a specific UDP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name
is added to the pipeline pack and can be use to accept messages using the
message_signer configuration option.

.. note::

    The UDP payload is not restricted to a single message; since the stream
    parser is being used multiple messages can be sent in a single payload.

Config:

- address (string):
    An IP address:port or Unix datagram socket file path on which this plugin
    will listen.
- signer:
    Optional TOML subsection. Section name consists of a signer name,
    underscore, and numeric version of the key.

    - hmac_key (string):
        The hash key used to sign the message.

.. versionadded:: 0.4

- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the raw input data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the stream on a byte delimiter.
    - regexp - splits the stream on a regexp delimiter.
    - message.proto - splits the stream on protobuf message boundaries.
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the message depending on the delimiter_location configuration.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of the message.
    - end - the regexp delimiter occurs at the end of the message (default).

.. versionadded:: 0.5

- net (string, optional, default: "udp")
    Network value must be one of: "udp", "udp4", "udp6", or "unixgram".

Example:

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

    [UdpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [UdpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [UdpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"

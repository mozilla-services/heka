.. _config_udp_input:

UDP Input
=========

Plugin Name: **UdpInput**

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

.. versionadded:: 0.5

- net (string, optional, default: "udp")
    Network value must be one of: "udp", "udp4", "udp6", or "unixgram".

.. versionadded:: 0.10

- set_hostname (boolean, default: false)
    Set Hostname field from remote address.

Example:

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"
    splitter = "HekaFramingSplitter"
    decoder = "ProtobufDecoder"

    [UdpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [UdpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [UdpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"

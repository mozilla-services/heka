.. _config_multidecoder:

MultiDecoder
============

Plugin Name: **MultiDecoder**

This decoder plugin allows you to specify an ordered list of delegate
decoders.  The MultiDecoder will pass the PipelinePack to be decoded to each
of the delegate decoders in turn until decode succeeds.  In the case of
failure to decode, MultiDecoder will return an error and recycle the message.

Config:

- subs ([]string):
    An ordered list of subdecoders to which the MultiDecoder will delegate.
    Each item in the list should specify another decoder configuration section
    by section name. Must contain at least one entry.

- log_sub_errors (bool):
    If true, the DecoderRunner will log the errors returned whenever a
    delegate decoder fails to decode a message. Defaults to false.

- cascade_strategy (string):
    Specifies behavior the MultiDecoder should exhibit with regard to
    cascading through the listed decoders. Supports only two valid values:
    "first-wins" and "all". With "first-wins", each decoder will be tried in
    turn until there is a successful decoding, after which decoding will be
    stopped. With "all", all listed decoders will be applied whether or not
    they succeed. In each case, decoding will only be considered to have
    failed if *none* of the sub-decoders succeed.

Here is a slightly contrived example where we have protocol buffer encoded
messages coming in over a TCP connection, with each message containin a single
nginx log line. Our MultiDecoder will run each message through two decoders,
the first to deserialize the protocol buffer and the second to parse the log
text:

.. code-block:: ini

    [TcpInput]
    address = ":5565"
    parser_type = "message.proto"
    decoder = "shipped-nginx-decoder"

    [shipped-nginx-decoder]
    type = "MultiDecoder"
    subs = ['ProtobufDecoder', 'nginx-access-decoder']
    cascade_strategy = "all"
    log_sub_errors = true

    [ProtobufDecoder]

    [nginx-access-decoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/nginx_access.lua"

        [nginx-access-decoder.config]
        type = "combined"
        user_agent_transform = true
        log_format = '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"'

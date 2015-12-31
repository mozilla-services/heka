.. _config_scribbledecoder:

Scribble Decoder
================

.. versionadded:: 0.5

Plugin Name: **ScribbleDecoder**

The ScribbleDecoder is a trivial decoder that makes it possible to set one or
more static field values on every decoded message. It is often used in
conjunction with another decoder (i.e. in a MultiDecoder w/ cascade_strategy
set to "all") to, for example, set the message type of every message to a
specific custom value after the messages have been decoded from Protocol
Buffers format. Note that this only supports setting the exact same value on
every message, if any dynamic computation is required to determine what the
value should be, or whether it should be applied to a specific message, a
:ref:`config_sandboxdecoder` using the provided `write_message` API call
should be used instead.

Config:

- message_fields:
    Subsection defining message fields to populate. Optional representation
    metadata can be added at the end of the field name using a pipe delimiter
    i.e. `host|ipv4 = "192.168.55.55"` will create Fields[Host] containing an
    IPv4 address. Adding a representation string to a standard message header
    name will cause it to be added as a user defined field, i.e. Payload|json
    will create Fields[Payload] with a json representation (see
    :ref:`field_variables`). Does not support Timestamp or Uuid.

Example (in MultiDecoder context)

.. code-block:: ini

        [AccesslogDecoder]
        type = "MultiDecoder"
        subs = ["AccesslogApacheDecoder", "EnvironmentScribbler"]
        cascade_strategy = "all"
        log_sub_errors = true

        [AccesslogApacheDecoder]
        type = "SandboxDecoder"
        filename = "lua_decoders/apache_access.lua"

            [AccesslogApacheDecoder.config]
            type = "combinedio"
            user_agent_transform = true
            user_agent_conditional = true
            log_format = '%h %l %u %t %m \"%U\" \"%q\" \"%H\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\" %T %I %O'

        [EnvironmentScribbler]
        type = "ScribbleDecoder"

            [EnvironmentScribbler.message_fields]
            Environment = "production"

You can also add static fields to messages decoded by a downstream heka instance
that forwards messages for further processing.

.. code-block:: ini

        [mytypedecoder]
        type = "MultiDecoder"
        subs = ["ProtobufDecoder", "mytype"]
        cascade_strategy = "all"
        log_sub_errors = true

        [ProtobufDecoder]

        [mytype]
        type = "ScribbleDecoder"

            [mytype.message_fields]
            Type = "MyType"

Message scribbling is commonly performed after the lua sandboxed decoders have been
applied. Otherwise the scribbled field may get discarded, depending on the
decoder implementation.

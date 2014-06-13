
ProtobufEncoder
===============

The ProtobufEncoder is used to serialize Heka message objects back into Heka's
standard protocol buffers format. This is the format that Heka uses to
communicate with other Heka instances, so one will always be included in your
Heka configuration whether specified or not. The ProtobufDecoder has no
configuration options.

The hekad protocol buffers message schema is defined in the `message.proto`
file in the `message` package.

Config:

- include_framing (bool):
    Whether or not to include protocol framing bits (useful for streaming) in the resulting output. Defaults to true.

Example:

.. code-block:: ini

    [ProtobufEncoder]

.. code-block:: ini

    [ProtobufEncoder]
    include_framing: false

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_

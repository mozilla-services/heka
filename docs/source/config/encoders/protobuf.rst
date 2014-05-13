
ProtobufEncoder
===============

The ProtobufEncoder is used to serialize Heka message objects back into Heka's
standard protocol buffers format. This is the format that Heka uses to
communicate with other Heka instances, so one will always be included in your
Heka configuration whether specified or not. The ProtobufDecoder has no
configuration options.

The hekad protocol buffers message schema in defined in the `message.proto`
file in the `message` package.

Example:

.. code-block:: ini

    [ProtobufEncoder]

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_


ProtobufDecoder
===============

The ProtobufDecoder is used for Heka message objects that have been serialized
into protocol buffers format. This is the format that Heka uses to communicate
with other Heka instances, so it is almost always a good idea to include one in
your Heka configuration. The ProtobufDecoder has no configuration options.

The hekad protocol buffers message schema in defined in the `message.proto`
file in the `message` package.

Example:

.. code-block:: ini

    [ProtobufDecoder]

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_

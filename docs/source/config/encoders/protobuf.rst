.. _config_protobufencoder:

Protobuf Encoder
================

Plugin Name: **ProtobufEncoder**

The ProtobufEncoder is used to serialize Heka message objects back into Heka's
standard protocol buffers format. This is the format that Heka uses to
communicate with other Heka instances, so one will always be included in your
Heka configuration using the default "ProtobufEncoder" name whether specified
or not.

The hekad protocol buffers message schema is defined in the `message.proto`
file in the `message` package.

Config:

<none>

Example:

.. code-block:: ini

    [ProtobufEncoder]

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_

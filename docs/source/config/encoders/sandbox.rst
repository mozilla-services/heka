
SandboxEncoder
==============

The SandboxEncoder provides an isolated execution environment for data parsing
and complex transformations without the need to recompile Heka. See
:ref:`sandbox`.

.. _sandboxencoder_settings:

Config:

- :ref:`config_common_sandbox_parameters`

- emits_protobuf (bool):
    This should be set to true if the encoder calls `inject_message` with a
    fully formed message table (see :ref:`inject_message(message_table)
    <inject_message_message_table>`) or if `write_message` is used (see
    :ref:`write_message <write_message>`). In each of these cases the decoder
    will be emitting a native Heka protocol buffer encoded message, and
    setting this to true will ensure that Heka handles the message framing
    correctly. Defaults to false.

Example

.. code-block:: ini

    [custom_json_encoder]
    type = "SandboxEncoder"
    filename = "path/to/custom_json_encoder.lua"

        [custom_json_encoder.config]
        msg_fields = ["field1", "field2"]

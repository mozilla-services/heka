.. _config_sandboxencoder:

Sandbox Encoder
===============

Plugin Name: **SandboxEncoder**

The SandboxEncoder provides an isolated execution environment for converting
messages into binary data without the need to recompile Heka. See
:ref:`sandbox`.

.. _sandboxencoder_settings:

Config:

- :ref:`config_common_sandbox_parameters`

Example

.. code-block:: ini

    [custom_json_encoder]
    type = "SandboxEncoder"
    filename = "path/to/custom_json_encoder.lua"

        [custom_json_encoder.config]
        msg_fields = ["field1", "field2"]

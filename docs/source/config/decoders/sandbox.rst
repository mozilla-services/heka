.. _config_sandboxdecoder:

Sandbox Decoder
===============

Plugin Name: **SandboxDecoder**

The SandboxDecoder provides an isolated execution environment for data parsing
and complex transformations without the need to recompile Heka. See
:ref:`sandbox`.

.. _sandboxdecoder_settings:

Config:

- :ref:`config_common_sandbox_parameters`

Example

.. code-block:: ini

    [sql_decoder]
    type = "SandboxDecoder"
    filename = "sql_decoder.lua"


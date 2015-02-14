.. _config_sandbox_input:

Sandbox Input
=============

.. versionadded:: 0.9

Plugin Name: **SandboxInput**

The SandboxInput provides a flexible execution environment for data ingestion
and transformation without the need to recompile Heka. Like all other sandboxes
it needs to implement a process_message function. However, it doesn't have to
return until shutdown. If you would like to implement a polling interface
process_message can return zero when complete and it will be called again the
next time TickerInterval fires (if ticker_interval was set to zero it
would simply exit after running once). See :ref:`sandbox`.

.. _sandboxinput_settings:

Config:

- All of the common input configuration parameters are ignored since the data
  processing (splitting and decoding) should happen in the plugin.
- :ref:`config_common_sandbox_parameters`
    - ``instruction_limit`` is always set to zero for SandboxInputs

Example

.. code-block:: ini

    [MemInfo]
    type = "SandboxInput"
    filename = "meminfo.lua"

    [MemInfo.config]
    path = "/proc/meminfo"


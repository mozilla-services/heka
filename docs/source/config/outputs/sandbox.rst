.. _config_sandbox_output:

Sandbox Output
==============

.. versionadded:: 0.9

Plugin Name: **SandboxOutput**

The SandboxOutput provides a flexible execution environment for data encoding
and transmission without the need to recompile Heka. See :ref:`sandbox`.

.. _sandboxoutput_settings:

Config:

- The common output configuration parameter 'encoder' is ignored since all data
  transformation should happen in the plugin.
- :ref:`config_common_sandbox_parameters`
- timer_event_on_shutdown (bool):
    True if the sandbox should have its timer_event function called on shutdown.

Example

.. code-block:: ini

    [SandboxFileOutput]
    type = "SandboxOutput"
    filename = "fileoutput.lua"

    [SandboxFileOutput.config]
    path = "mylog.txt"


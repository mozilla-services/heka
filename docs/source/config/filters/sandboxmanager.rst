
SandboxManagerFilter
====================

The SandboxManagerFilter provides dynamic control (start/stop) of sandbox
filters in a secure manner without stopping the Heka daemon. Commands are sent
to a SandboxManagerFilter using a signed Heka message. The intent is to have
one manager per access control group each with their own message signing key.
Users in each group can submit a signed control message to manage any filters
running under the associated manager.  A signed message is not an enforced
requirement but it is highly recommended in order to restrict access to this
functionality.

SandboxManagerFilter Settings
-----------------------------

- :ref:`config_common_filter_parameters`

- working_directory (string):
    The directory where the filter configurations, code, and states are
    preserved.  The directory can be unique or shared between sandbox managers
    since the filter names are unique per manager. Defaults to a directory in
    ${BASE_DIR}/sbxmgrs with a name generated from the plugin name.

- module_directory (string):
    The directory where 'require' will attempt to load the external Lua
    modules from.  Defaults to ${SHARE_DIR}/lua_modules.

- max_filters (uint):
    The maximum number of filters this manager can run.

.. versionadded:: 0.5
- memory_limit (uint):
    The number of bytes managed sandboxes are allowed to consume before being
    terminated (max 8MiB, default max).

- instruction_limit (uint):
    The number of instructions managed sandboxes are allowed the execute during 
    the process_message/timer_event functions before being terminated (max 1M,
    default max).

- output_limit (uint):
    The number of bytes managed sandbox output buffers can hold before before 
    being terminated (max 63KiB, default max).  Anything less than 64B is set to
    64B.

Example

.. code-block:: ini

    [OpsSandboxManager]
    type = "SandboxManagerFilter"
    message_signer = "ops"
    # message_matcher = "Type == 'heka.control.sandbox'" # automatic default setting
    max_filters = 100

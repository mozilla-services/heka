
SandboxEncoder
==============

The SandboxEncoder provides an isolated execution environment for data parsing
and complex transformations without the need to recompile Heka. See
:ref:`sandbox`.

.. _sandboxencoder_settings:

Config:

- script_type (string):
    The language the sandbox is written in. Currently the only valid option is
    'lua'.

- filename (string):
    The path to the sandbox code; if specified as a relative path it will be
    appended to Heka's global share_dir.

- preserve_data (bool):
    True if the sandbox global data should be preserved/restored on Heka
    shutdown/startup.

- memory_limit (uint):
    The number of bytes the sandbox is allowed to consume before being
    terminated (max 8MiB, default max).

- instruction_limit (uint):
    The number of instructions the sandbox is allowed the execute during the
    process_message function before being terminated (max 1M, default max).

- output_limit (uint):
    The number of bytes the sandbox output buffer can hold before before being
    terminated (max 63KiB, default max).  Anything less than 64B is set to
    64B.

- module_directory (string):
    The directory where 'require' will attempt to load the external Lua
    modules from.  Defaults to ${SHARE_DIR}/lua_modules.

- config (object):
    A map of configuration variables available to the sandbox via read_config.
    The map consists of a string key with: string, bool, int64, or float64
    values.

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
    script_type = "lua"
    filename = "custom_json_decoder.lua"

        [custom_json_encoder.config]
        msg_fields = ["field1", "field2"]

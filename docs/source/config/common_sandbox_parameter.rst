.. _config_common_sandbox_parameters:

Common Sandbox Parameters
=========================
These are the configuration options that are universally available to all
Sandbox plugins. The are consumed by Heka when it initializes the plugin.

- script_type (string):
    The language the sandbox is written in. Currently the only valid option is
    'lua' which is the default.

- filename (string):
    The path to the sandbox code; if specified as a relative path it will be
    appended to Heka's global share_dir.

- preserve_data (bool):
    True if the sandbox global data should be preserved/restored on plugin
    shutdown/startup. When true this works in conjunction with a global Lua
    _PRESERVATION_VERSION variable which is examined during restoration;
    if the previous version does not match the current version the restoration
    will be aborted and the sandbox will start cleanly. _PRESERVATION_VERSION
    should be incremented any time an incompatible change is made to the global
    data schema. If no version is set the check will always succeed and a 
    version of zero is assumed.

- memory_limit (uint):
    The number of bytes the sandbox is allowed to consume before being
    terminated (default 8MiB).

- instruction_limit (uint):
    The number of instructions the sandbox is allowed to execute during the
    process_message/timer_event functions before being terminated (default 1M).

- output_limit (uint):
    The number of bytes the sandbox output buffer can hold before being
    terminated (default 63KiB). Warning: messages exceeding 64KiB will generate
    an error and be discarded by the standard output plugins (File, TCP, UDP)
    since they exceed the maximum message size.

- module_directory (string):
    The directory where 'require' will attempt to load the external Lua
    modules from.  Defaults to ${SHARE_DIR}/lua_modules.

- config (object):
    A map of configuration variables available to the sandbox via read_config.
    The map consists of a string key with: string, bool, int64, or float64
    values.

.. _sandboxdecoder:

Sandbox Decoder
===============

The sandbox decoder provides an isolated execution environment for data parsing
and complex transformations without the need to recompile Heka.

If a value for `timestamp_field` is specified, the sandbox decoder
will parse a timestamp and set the Timestamp field in the message.

.. _sandboxdecoder_settings:

SandboxDecoder Settings
-----------------------

- script_type (string): 
    The language the sandbox is written in.  Currently the only valid option is 'lua'.

- filename (string): 
    The path to the sandbox code; if specified as a relative path it will be appended to Heka's global base_dir.

- memory_limit (uint): 
    The number of bytes the sandbox is allowed to consume before being terminated (max 8MiB, default max).

- instruction_limit (uint): 
    The number of instructions the sandbox is allowed the execute during the process_message function before being terminated (max 1M, default max).

- output_limit (uint): 
    The number of bytes the sandbox output buffer can hold before before being terminated (max 63KiB, default max).  Anything less than 1KiB will default to 1KiB.

- config (object):
    A map of configuration variables available to the sandbox via read_config.  The map consists of a string key with: string, bool, int64, or float64 values.

- timestamp_field (string):
    The field name where a string formatted datetime stamp can be found  Default is the empty string and no timestamp will be parsed.

- timestamp_layout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documentation <http://golang.org/pkg/time/#pkg-constants>`_.

 - timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`.

Example

.. code-block:: ini

    [sql_decoder]
    type = "SandboxDecoder"
    script_type = "lua"
    filename = "sql_decoder.lua"

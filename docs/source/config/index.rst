.. _configuration:

=================
Configuring hekad
=================

.. start-hekad-config

A hekad configuration file specifies what inputs, decoders, filters, and
outputs will be loaded. The configuration file is in `TOML
<https://github.com/mojombo/toml>`_ format. TOML looks very similar to INI
configuration formats, but with slightly more rich data structures and nesting
support.

If hekad's config file is specified to be a directory, all files will be
loaded and merged into a single config. Merging will happen in alphabetical
order, settings specified later in the merge sequence will win conflicts. All
files in the folder must be valid TOML configuration or hekad will not start.

The config file is broken into sections, with each section representing a
single instance of a plugin. The section name specifies the name of the
plugin, and the "type" parameter specifies the plugin type; this must match
one of the types registered via the `pipeline.RegisterPlugin` function. For
example, the following section describes a plugin named "tcp:5565", an
instance of Heka's plugin type "TcpInput":

.. code-block:: ini

    [tcp:5565]
    type = "TcpInput"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"
    address = ":5565"

If you choose a plugin name that also happens to be a plugin type name, then
you can omit the "type" parameter from the section and the specified name will
be used as the type. Thus, the following section describes a plugin named
"TcpInput", also of type "TcpInput":

.. code-block:: ini

    [TcpInput]
    address = ":5566"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

Note that it's fine to have more than one instance of the same plugin type, as
long as their configurations don't interfere with each other.

Any values other than "type" in a section, such as "address" in the above
examples, will be passed through to the plugin for internal configuration (see
:ref:`plugin_config`).

If a plugin fails to load during startup, hekad will exit at startup. When
hekad is running, if a plugin should fail (due to connection loss, inability
to write a file, etc.) then hekad will either shut down or restart the plugin
if the plugin supports restarting. When a plugin is restarting, hekad will
likely stop accepting messages until the plugin resumes operation (this
applies only to filters/output plugins).

Plugins specify that they support restarting by implementing the Restarting
interface (see :ref:`restarting_plugins`). Plugins supporting Restarting can
have :ref:`their restarting behavior configured <configuring_restarting>`.

An internal diagnostic runner runs every 30 seconds to sweep the packs used
for messages so that possible bugs in heka plugins can be reported and pinned
down to a likely plugin(s) that failed to properly recycle the pack.

.. end-hekad-config

Global configuration options
============================

You can optionally declare a `[hekad]` section in your configuration file to
configure some global options for the heka daemon.

Config:

- cpuprof (string `output_file`):
    Turn on CPU profiling of hekad; output is logged to the `output_file`.

- max_message_loops (uint):
    The maximum number of times a message can be re-injected into the system.
    This is used to prevent infinite message loops from filter to filter;
    the default is 4.

- max_process_inject (uint):
    The maximum number of messages that a sandbox filter's ProcessMessage
    function can inject in a single call; the default is 1.

- max_process_duration (uint64):
    The maximum number of nanoseconds that a sandbox filter's ProcessMessage
    function can consume in a single call before being terminated; the default
    is 100000.

- max_timer_inject (uint):
    The maximum number of messages that a sandbox filter's TimerEvent
    function can inject in a single call; the default is 10.

- max_pack_idle (string):
    A time duration string (e.x. "2s", "2m", "2h") indicating how long a
    message pack can be 'idle' before its considered leaked by heka. If too
    many packs leak from a bug in a filter or output then heka will eventually
    halt. This setting indicates when that is considered to have occurred.

- maxprocs (int):
    Enable multi-core usage; the default is 1 core. More cores will generally
    increase message throughput. Best performance is usually attained by
    setting this to 2 x (number of cores). This assumes each core is
    hyper-threaded.

- memprof (string `output_file`):
    Enable memory profiling; output is logged to the `output_file`.

- poolsize (int):
    Specify the pool size of maximum messages that can exist; default is 100
    which is usually sufficient and of optimal performance.

- plugin_chansize (int):
    Specify the buffer size for the input channel for the various Heka
    plugins. Defaults to 50, which is usually sufficient and of optimal
    performance.

- base_dir (string):
    Base working directory Heka will use for persistent storage through
    process and server restarts. The hekad process must have read and write
    access to this directory. Defaults to `/var/cache/hekad` (or
    `c:\var\cache\hekad` on Windows).

- share_dir (string):
    Root path of Heka's "share directory", where Heka will expect to find
    certain resources it needs to consume. The hekad process should have read-
    only access to this directory. Defaults to `/usr/share/heka` (or
    `c:\\usr\\share\\heka` on Windows).

.. versionadded:: 0.6
- sample_denominator (int):
    Specifies the denominator of the sample rate Heka will use when computing
    the time required to perform certain operations, such as for the
    ProtobufDecoder to decode a message, or the router to compare a message
    against a message matcher. Defaults to 1000, i.e. duration will be
    calculated for one message out of 1000.

.. versionadded:: 0.6
- pidfile (string):
    Optionally specify the location of a pidfile where the process id of
    the running hekad process will be written. The hekad process must have
    read and write access to the parent directory (which is not automatically
    created). On a successful exit the pidfile will be removed. If the path
    already exists the contained pid will be checked for a running process.
    If one is found, the current process will exit with an error.

Example hekad.toml file
=======================

.. start-hekad-toml

.. code-block:: ini

    [hekad]
    cpuprof = "/var/log/hekad/cpuprofile.log"
    decoder_poolsize = 10
    max_message_loops = 4
    max_process_inject = 10
    max_timer_inject  = 10
    maxprocs = 10
    memprof = "/var/log/hekad/memprof.log"
    plugin_chansize = 10
    poolsize = 100

    # Listens for Heka messages on TCP port 5565.
    [TcpInput]
    address = ":5565"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

    # Writes output from `CounterFilter`, `lua_sandbox`, and Heka's internal
    # reports to stdout.
    [debug]
    type = "LogOutput"
    message_matcher = "Type == 'heka.counter-output' || Type == 'heka.all-report' || Type == 'heka.sandbox-output'"

    # Counts throughput of messages sent from a Heka load testing tool.
    [CounterFilter]
    message_matcher = "Type == 'hekabench' && EnvVersion == '0.8'"
    output_timer = 1

    # Defines a sandboxed filter that will be written in Lua.
    [lua_sandbox]
    type = "SandboxFilter"
    message_matcher = "Type == 'hekabench' && EnvVersion == '0.8'"
    output_timer = 1
    script_type = "lua"
    preserve_data = true
    filename = "lua/sandbox.lua"

.. end-hekad-toml

.. _configuring_restarting:

.. start-restarting

Configuring Restarting Behavior
===============================

Plugins that support being restarted have a set of options that govern how the
restart is handled. If preferred, the plugin can be configured to not restart
at which point hekad will exit, or it could be restarted only 100 times, or
restart attempts can proceed forever.

Adding the restarting configuration is done by adding a config section to the
plugins' config called `retries`. A small amount of jitter will be added to
the delay between restart attempts.

Config:

- max_jitter (string):
    The longest jitter duration to add to the delay between restarts. Jitter
    up to 500ms by default is added to every delay to ensure more even restart
    attempts over time.
- max_delay (string):
    The longest delay between attempts to restart the plugin. Defaults to 30s
    (30 seconds).
- delay (string):
    The starting delay between restart attempts. This value will be the
    initial starting delay for the exponential back-off, and capped to be no
    larger than the `max_delay`. Defaults to 250ms.
- max_retries (int):
    Maximum amount of times to attempt restarting the plugin before giving up
    and shutting down hekad. Use 0 for no retry attempt, and -1 to continue
    trying forever (note that this will cause hekad to halt possibly forever
    if the plugin cannot be restarted).

Example (UdpInput does not actually support nor need restarting, illustrative
purposes only):

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

    [UdpInput.retries]
    max_delay = 30s
    delay = 250ms
    max_retries = 5

.. end-restarting

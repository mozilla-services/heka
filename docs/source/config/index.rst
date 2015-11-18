.. _configuration:

=================
Configuring hekad
=================

.. start-hekad-config

A hekad configuration file specifies what inputs, splitters, decoders,
filters, encoders, and outputs will be loaded. The configuration file is in
`TOML <https://github.com/mojombo/toml>`_ format. TOML looks very similar to
INI configuration formats, but with slightly more rich data structures and
nesting support.

If hekad's config file is specified to be a directory, all contained files
with a filename ending in ".toml" will be loaded and merged into a single
config. Files that don't end with ".toml" will be ignored. Merging will happen
in alphabetical order, settings specified later in the merge sequence will win
conflicts.

The config file is broken into sections, with each section representing a
single instance of a plugin. The section name specifies the name of the
plugin, and the "type" parameter specifies the plugin type; this must match
one of the types registered via the `pipeline.RegisterPlugin` function. For
example, the following section describes a plugin named "tcp:5565", an
instance of Heka's plugin type "TcpInput":

.. code-block:: ini

    [tcp:5565]
    type = "TcpInput"
    splitter = "HekaFramingSplitter"
    decoder = "ProtobufDecoder"
    address = ":5565"

If you choose a plugin name that also happens to be a plugin type name, then
you can omit the "type" parameter from the section and the specified name will
be used as the type. Thus, the following section describes a plugin named
"TcpInput", also of type "TcpInput":

.. code-block:: ini

    [TcpInput]
    address = ":5566"
    splitter = "HekaFramingSplitter"
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
interface (see :ref:`restarting_plugin`). Plugins supporting Restarting can
have :ref:`their restarting behavior configured <configuring_restarting>`.

An internal diagnostic runner runs every 30 seconds to sweep the packs used
for messages so that possible bugs in heka plugins can be reported and pinned
down to a likely plugin(s) that failed to properly recycle the pack.

.. end-hekad-config

.. _hekad_global_config_options:

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
    Specify the pool size of maximum messages that can exist. Default is 100.

- plugin_chansize (int):
    Specify the buffer size for the input channel for the various Heka
    plugins. Defaults to 30.

- base_dir (string):
    Base working directory Heka will use for persistent storage through
    process and server restarts. The hekad process must have read and write
    access to this directory. Defaults to `/var/cache/hekad` (or
    `c:\\var\\cache\\hekad` on Windows).

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

- pid_file (string):
    Optionally specify the location of a pidfile where the process id of
    the running hekad process will be written. The hekad process must have
    read and write access to the parent directory (which is not automatically
    created). On a successful exit the pidfile will be removed. If the path
    already exists the contained pid will be checked for a running process.
    If one is found, the current process will exit with an error.

.. versionadded:: 0.9

- hostname (string):
    Specifies the hostname to use whenever Heka is asked to provide the local
    host's hostname. Defaults to whatever is provided by Go's `os.Hostname()`
    call.

- max_message_size (uint32):
    The maximum size (in bytes) of message can be sent during processing.
    Defaults to 64KiB.

.. versionadded:: 0.10

- log_flags (int):
    Control the prefix for STDOUT and STDERR logs. Common values are 3 (date
    and time, the default) or 0 (no prefix). See
    `https://golang.org/pkg/log/#pkg-constants Go documentation`_ for details.

- full_buffer_max_retries (int):
    When Heka shuts down due to a buffer filling to capacity, the next time
    Heka starts it will delay startup briefly to give the buffer a chance to
    drain, to alleviate the back-pressure. This setting specifies the maximum
    number of intervals (max 1s in duration) Heka should wait for the buffer
    size to get below 90% of capacity before deciding that the issue is not
    resolved and continuing startup (or shutting down).

Example hekad.toml file
=======================

.. start-hekad-toml

.. code-block:: ini

    [hekad]
    maxprocs = 4

    # Heka dashboard for internal metrics and time series graphs
    [Dashboard]
    type = "DashboardOutput"
    address = ":4352"
    ticker_interval = 15

    # Email alerting for anomaly detection
    [Alert]
    type = "SmtpOutput"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'alert'"
    send_from = "acme-alert@example.com"
    send_to = ["admin@example.com"]
    auth = "Plain"
    user = "smtp-user"
    password = "smtp-pass"
    host = "mail.example.com:25"
    encoder = "AlertEncoder"

    # User friendly formatting of alert messages
    [AlertEncoder]
    type = "SandboxEncoder"
    filename = "lua_encoders/alert.lua"

    # Nginx access log reader
    [AcmeWebserver]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = 'access\.log'
    decoder = "CombinedNginxDecoder"

    # Nginx access 'combined' log parser
    [CombinedNginxDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/nginx_access.lua"

        [CombinedNginxDecoder.config]
        user_agent_transform = true
        user_agent_conditional = true
        type = "combined"
        log_format = '$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"'

    # Collection and visualization of the HTTP status codes
    [AcmeHTTPStatus]
    type = "SandboxFilter"
    filename = "lua_filters/http_status.lua"
    ticker_interval = 60
    preserve_data = true
    message_matcher = "Logger == 'AcmeWebserver'"

        # rate of change anomaly detection on column 1 (HTTP 200)
        [AcmeHTTPStatus.config]
        anomaly_config = 'roc("HTTP Status", 1, 15, 0, 1.5, true, false)'

.. end-hekad-toml

Using Environment Variables
===========================

If you wish to use environmental variables in your config files as a way to
configure values, you can simply use ``%ENV[VARIABLE_NAME]`` and the text will
be replaced with the value of the environmental variable ``VARIABLE_NAME``.

Example:

.. code-block:: ini

    [AMQPInput]
    url = "amqp://%ENV[USER]:%ENV[PASSWORD]@rabbitmq/"
    exchange = "testout"
    exchangeType = "fanout"


.. start-restarting

.. _configuring_restarting:

Configuring Restarting Behavior
===============================

Plugins that support being restarted have a set of options that govern how a
restart is handled if they exit with an error.  If preferred, the plugin can be
configured to not restart, or it could be restarted only 100 times, or restart
attempts can proceed forever.
Once the `max_retries` have been exceeded the plugin will be unregistered,
potentially triggering hekad to shutdown (depending on the plugin's `can_exit`
configuration).

Adding the restarting configuration is done by adding a config section to a
plugin's configuration called `retries`. A small amount of jitter will be
added to the delay between restart attempts.

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
    and exiting the plugin. Use 0 for no retry attempt, and -1 to continue
    trying forever (note that this will cause hekad to halt possibly forever
    if the plugin cannot be restarted). Defaults to -1.

Example:

.. code-block:: ini

    [AMQPOutput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchange_type = "fanout"
    message_matcher = 'Logger == "TestWebserver"'

    [AMQPOutput.retries]
    max_delay = "30s"
    delay = "250ms"
    max_retries = 5

.. end-restarting

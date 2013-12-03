.. _configuration:

=================
Configuring hekad
=================

.. start-hekad-config

A hekad configuration file specifies what inputs, decoders, filters,
and outputs will be loaded. The configuration file is in `TOML
<https://github.com/mojombo/toml>`_ format. TOML looks is very similar
to INI configuration formats, but with slightly more rich data
structures and nesting support.

The config file is broken into sections, with each section representing
a single instance of a plugin. The section name specifies the name of
the plugin, and the "type" parameter specifies the plugin type; this
must match one of the types registered via the
`pipeline.RegisterPlugin` function. For example, the following section
describes a plugin named "tcp:5565", an instance of Heka's plugin type
"TcpInput":

.. code-block:: ini

    [tcp:5565]
    type = "TcpInput"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"
    address = ":5565"

If you choose a plugin name that also happens to be a plugin type name,
then you can omit the "type" parameter from the section and the
specified name will be used as the type. Thus, the following section
describes a plugin named "TcpInput", also of type "TcpInput":

.. code-block:: ini

    [TcpInput]
    address = ":5566"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

Note that it's fine to have more than one instance of the same plugin
type, as long as their configurations don't interfere with each other.

Any values other than "type" in a section, such as "address" in the
above examples, will be passed through to the plugin for internal
configuration (see :ref:`plugin_config`).

If a plugin fails to load during startup, hekad will exit at startup.
When hekad is running, if a plugin should fail (due to connection loss,
inability to write a file, etc.) then hekad will either shut down or
restart the plugin if the plugin supports restarting. When a plugin is
restarting, hekad will likely stop accepting messages until the plugin
resumes operation (this applies only to filters/output plugins).

Plugins specify that they support restarting by implementing the
Restarting interface (see :ref:`restarting_plugins`). Plugins
supporting Restarting can have :ref:`their restarting behavior
configured <configuring_restarting>`.

An internal diagnostic runner runs every 30 seconds to sweep the packs
used for messages so that possible bugs in heka plugins can be reported
and pinned down to a likely plugin(s) that failed to properly recycle
the pack.

.. end-hekad-config

Global configuration options
============================

You can optionally declare a `[hekad]` section in your configuration
file to configure some global options for the heka daemon.

Parameters:

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

- decoder_poolsize (int):
    Specify the number of decoder sets to spin up for use converting input
    data to Heka's Message objects. Default is 4, optimal value is variable,
    depending on number of total running plugins, number of expected
    concurrent connections, amount of expected traffic, and number of
    available cores on the host.

- plugin_chansize (int):
    Specify the buffer size for the input channel for the various Heka
    plugins. Defaults to 50, which is usually sufficient and of optimal
    performance.

- base_dir (string):
    Base working directory Heka will use for persistent storage through
    process and server restarts. Defaults to `/var/cache/hekad` (or
    `c:\var\cache\hekad` on windows).


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
    memory_limit = 32767
    instruction_limit = 1000
    output_limit = 1024

.. end-hekad-toml

Common Roles
============
.. start-roles

- **Agent** - Single default filter that passes all messages directly to
  another `hekad` daemon on a separate machine configured as an
  Router.
- **Aggregator** - Runs filters that can roll-up statistics (similar to
  statsd), and handles aggregating similar messages before saving them
  to a back-end directly or possibly forwarding them to a `hekad`
  router.
- **Router** - Collects input messages from multiple sources (including
  other `hekad` daemons acting as Agents), rolls up stats, and routes
  messages to appropriate back-ends.

.. end-roles

.. _hekad_command_line_options:

Command Line Options
====================

.. start-options

``-version``
    Output the version number, then exit.

``-config`` `config_file`
    Specify the configuration file to use; the default is /etc/hekad.toml.  (See hekad.config(5).)


.. end-options

.. start-restarting

.. _configuring_restarting:

Configuring Restarting Behavior
===============================

Plugins that support being restarted have a set of options that govern
how the restart is handled. If preferred, the plugin can be configured
to not restart at which point hekad will exit, or it could be restarted
only 100 times, or restart attempts can proceed forever.

Adding the restarting configuration is done by adding a config section
to the plugins' config called `retries`. A small amount of jitter will
be added to the delay between restart attempts.

Parameters:

- max_jitter (string):
    The longest jitter duration to add to the delay between restarts. Jitter
    up to 500ms by default is added to every delay to ensure more even
    restart attempts over time.
- max_delay (string):
    The longest delay between attempts to restart the plugin. Defaults to
    30s (30 seconds).
- delay (string):
    The starting delay between restart attempts. This value will be the
    initial starting delay for the exponential back-off, and capped to
    be no larger than the `max_delay`. Defaults to 250ms.
- max_retries (int):
    Maximum amount of times to attempt restarting the plugin before giving
    up and shutting down hekad. Use 0 for no retry attempt, and -1 to
    continue trying forever (note that this will cause hekad to halt
    possibly forever if the plugin cannot be restarted).

Example (UdpInput does not actually support nor need restarting,
illustrative purposes only):

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

.. start-inputs

Inputs
======

.. _config_amqp_input:

AMQPInput
---------

Connects to a remote AMQP broker (RabbitMQ) and retrieves messages from the
specified queue. As AMQP is dynamically programmable, the broker topology
needs to be specified in the plugin configuration.

Parameters:

- URL (string):
    An AMQP connection string formatted per the `RabbitMQ URI Spec
    <http://www.rabbitmq.com/uri-spec.html>`_.
- Exchange (string):
    AMQP exchange name
- ExchangeType (string):
    AMQP exchange type (`fanout`, `direct`, `topic`, or `headers`).
- ExchangeDurability (bool):
    Whether the exchange should be configured as a durable exchange. Defaults
    to non-durable.
- ExchangeAutoDelete (bool):
    Whether the exchange is deleted when all queues have finished and there
    is no publishing. Defaults to auto-delete.
- RoutingKey (string):
    The message routing key used to bind the queue to the exchange. Defaults
    to empty string.
- PrefetchCount (int):
    How many messages to fetch at once before message acks are sent. See
    `RabbitMQ performance measurements <http://www.rabbitmq.com/blog/2012/04/25/rabbitmq-performance-measurements-part-2/>`_
    for help in tuning this number. Defaults to 2.
- Queue (string):
    Name of the queue to consume from, an empty string will have the broker
    generate a name for the queue. Defaults to empty string.
- QueueDurability (bool):
    Whether the queue is durable or not. Defaults to non-durable.
- QueueExclusive (bool):
    Whether the queue is exclusive (only one consumer allowed) or not.
    Defaults to non-exclusive.
- QueueAutoDelete (bool):
    Whether the queue is deleted when the last consumer un-subscribes.
    Defaults to auto-delete.
- Decoder (string):
    Decoder name used to transform a raw message body into a structured hekad
    message. Must be a decoder appropriate for the messages that come in from
    the exchange. If accepting messages that have been generated by an
    AMQPOutput in another Heka process then this should be a
    :ref:`config_protobuf_decoder` instance.

Since many of these parameters have sane defaults, a minimal configuration to
consume serialized messages would look like:

.. code-block:: ini

    [AMQPInput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchangeType = "fanout"

Or if using a PayloadRegexDecoder to parse OSX syslog messages may look like:

.. code-block:: ini

    [AMQPInput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchangeType = "fanout"
    decoder = "logparser"

    [logparser]
    type = "MultiDecoder"
    order = ["logline", "leftovers"]

      [logparser.subs.logline]
      type = "PayloadRegexDecoder"
      MatchRegex = '\w+ \d+ \d+:\d+:\d+ \S+ (?P<Reporter>[^\[]+)\[(?P<Pid>\d+)](?P<Sandbox>[^:]+)?: (?P Remaining>.*)'

        [logparser.subs.logline.MessageFields]
        Type = "amqplogline"
        Hostname = "myhost"
        Reporter = "%Reporter%"
        Remaining = "%Remaining%"
        Logger = "%Logger%"
        Payload = "%Remaining%"

      [leftovers]
      type = "PayloadRegexDecoder"
      MatchRegex = '.*'

        [leftovers.MessageFields]
        Type = "drop"
        Payload = ""

.. _config_udp_input:

UdpInput
--------

Listens on a specific UDP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name
is added to the pipeline pack and can be use to accept messages using the
message_signer configuration option.

.. note::

    The UDP payload is not restricted to a single message; since the stream
    parser is being used multiple messages can be sent in a single payload.

Parameters:

- address (string):
    An IP address:port on which this plugin will listen.
- signer:
    Optional TOML subsection. Section name consists of a signer name,
    underscore, and numeric version of the key.

    - hmac_key (string):
        The hash key used to sign the message.

.. versionadded:: 0.4

- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the raw input data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the stream on a byte delimiter.
    - regexp - splits the stream on a regexp delimiter.
    - message.proto - splits the stream on protobuf message boundaries.
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the message depending on the delimiter_location configuration.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of the message.
    - end - the regexp delimiter occurs at the end of the message (default).

Example:

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

    [UdpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [UdpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [UdpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"


.. _config_tcp_input:

TcpInput
--------

Listens on a specific TCP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name
is added to the pipeline pack and can be use to accept messages using the
message_signer configuration option.

Parameters:

- address (string):
    An IP address:port on which this plugin will listen.
- signer:
    Optional TOML subsection. Section name consists of a signer name,
    underscore, and numeric version of the key.

    - hmac_key (string):
        The hash key used to sign the message.

.. versionadded:: 0.4

- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the raw input data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the stream on a byte delimiter.
    - regexp - splits the stream on a regexp delimiter.
    - message.proto - splits the stream on protobuf message boundaries.
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the message depending on the delimiter_location configuration.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of the message.
    - end - the regexp delimiter occurs at the end of the message (default).

Example:

.. code-block:: ini

    [TcpInput]
    address = ":5565"
    parser_type = "message.proto"
    decoder = "ProtobufDecoder"

    [TcpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [TcpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [TcpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"


.. _config_logfile_input:

LogfileInput
------------

Tails a single log file, creating a message for each line in the file being
monitored. Files are read in their entirety, and watched for changes. This
input gracefully handles log rotation via the file moving but may lose a few
log lines if using the "truncation" method of log rotation. It's recommended
to use log rotation schemes that move the file to another location to avoid
possible loss of log lines.

In the event the log file does not currently exist, it will be placed in an
internal discover list, and checked for existence every `discover_interval`
milliseconds (5000ms or 5s by default).

A single LogfileInput can only be used to read a single file. If you have
multiple identical files spread across multiple directories (e.g. a
`/var/log/hosts/<HOSTNAME>/app.log` structure, where each <HOSTNAME> folder
contains a log file originating from a separate host), you'll want to use the
:ref:`config_logfile_directory_manager_input`.

Parameters:

- logfile (string):
    Each LogfileInput can have a single logfile to monitor.
- hostname (string):
    The hostname to use for the messages, by default this will be the
    machines qualified hostname. This can be set explicitly to ensure
    its the correct name in the event the machine has multiple
    interfaces/hostnames.
- discover_interval (int):
    During logfile rotation, or if the logfile is not originally
    present on the system, this interval is how often the existence of
    the logfile will be checked for. The default of 5 seconds is
    usually fine. This interval is in milliseconds.
- stat_interval (int):
    How often the file descriptors for each file should be checked to
    see if new log data has been written. Defaults to 500 milliseconds.
    This interval is in milliseconds.
- logger (string):
    Each LogfileInput may specify a logger name to use in the case an
    error occurs during processing of a particular line of logging
    text.  By default, the logger name is set to the logfile name.
- use_seek_journal (bool):
    Specifies whether to use a seek journal to keep track of where we are
    in a file to be able to resume parsing from the same location upon
    restart. Defaults to true.
- seek_journal_name (string):
    Name to use for the seek journal file, if one is used. Only refers to
    the file name itself, not the full path; Heka will store all seek
    journals in a `seekjournal` folder relative to the Heka base directory.
    Defaults to a sanitized version of the `logger` value (which itself
    defaults to the filesystem path of the input file). This value is
    ignored if `use_seek_journal` is set to false.
- resume_from_start (bool):
    When heka restarts, if a logfile cannot safely resume reading from
    the last known position, this flag will determine whether hekad
    will force the seek position to be 0 or the end of file. By
    default, hekad will resume reading from the start of file.

.. versionadded:: 0.4

- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the parsed data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the log on a byte delimiter (default).
    - regexp - splits the log on a regexp delimiter.
    - message.proto - splits the log on protobuf message boundaries
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the log line depending on the delimiter_location configuration.
    Note: when a start delimiter is used the last line in the file will not be
    processed (since the next record defines its end) until the log is rolled.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of a log line.
    - end - the regexp delimiter occurs at the end of the log line (default).

.. code-block:: ini

    [LogfileInput]
    logfile = "/var/log/opendirectoryd.log"
    logger = "opendirectoryd"

.. code-block:: ini

    [LogfileInput]
    logfile = "/var/log/opendirectoryd.log"

.. _config_logfile_directory_manager_input:

LogfileDirectoryManagerInput
----------------------------

Scans for log files in a globbed directory path and when a new file matching
the specified path is discovered it will start an instance of the LogfileInput
plugin to process it. Each LogfileInput will inherit its configuration from
the manager's settings with the logfile property properly adjusted.

Parameters: (identical to LogfileInput with the following exceptions)

- logfile (string):
    A path with a globbed directory component pointing to a common (statically
    named) log file. Note that only directories can be globbed; the file itself
    must have the same name in each directory.
- seek_journal_name (string):
    With a LogfileInput it is possible to specify a particular name for the
    seek journal file that will be used. This is not possible with the
    LogfileDirectoryManagerInput; the seek_journal_name will always be auto-
    generated, and any attempt to specify a hard coded seek_journal_name will
    be treated as a configuration error.
- ticker_interval (uint):
    Time interval (in seconds) between directory scans for new log files.
    Defaults to 0 (only scans once on startup).

.. code-block:: ini

    [vhosts]
    type = "LogfileDirectoryManagerInput"
    logfile = "/var/log/vhost/*/apache.log"

.. note::

    The spawned LogfileInput plugins are named `manager_name`-`logfile` i.e.,

    - vhosts-/var/log/www/apache.log
    - vhosts-/var/log/internal/apache.log

.. _config_statsd_input:

StatsdInput
-----------

Listens for `statsd protocol <https://github.com/b/statsd_spec>`_ `counter`,
`timer`, or `gauge` messages on a UDP port, and generates `Stat` objects that
are handed to a `StatAccumulator` for aggregation and processing.

Parameters:

- address (string):
    An IP address:port on which this plugin will expose a statsd server.
    Defaults to "127.0.0.1:8125".
- stat_accum_name (string):
    Name of a StatAccumInput instance that this StatsdInput will use as its
    StatAccumulator for submitting received stat values. Defaults to
    "StatAccumInput".

Example:

.. code-block:: ini

    [StatsdInput]
    address = ":8125"
    stat_accum_input = "custom_stat_accumulator"

.. _config_stat_accum_input:

StatAccumInput
--------------

Provides an implementation of the `StatAccumulator` interface which other
plugins can use to submit `Stat` objects for aggregation and roll-up.
Accumulates these stats and then periodically emits a "stat metric" type
message containing aggregated information about the stats received since the
last generated message.

Parameters:

- emit_in_payload (bool):
    Specifies whether or not the aggregated stat information should be emitted
    in the payload of the generated messages, in the format accepted by the
    `carbon <http://graphite.wikidot.com/carbon>`_ portion of the `graphite
    <http://graphite.wikidot.com/>`_ graphing software. Defaults to true.
- emit_in_fields (bool):
    Specifies whether or not the aggregated stat information should be emitted
    in the message fields of the generated messages. Defaults to false. *NOTE*:
    At least one of 'emit_in_payload' or 'emit_in_fields' *must* be true or it
    will be considered a configuration error and the input won't start.
- percent_threshold (int):
    Percent threshold to use for computing "upper_N%" type stat values.
    Defaults to 90.
- ticker_interval (uint):
    Time interval (in seconds) between generated output messages.
    Defaults to 10.
- message_type (string):
    String value to use for the `Type` value of the emitted stat messages.
    Defaults to "heka.statmetric".

.. _config_process_input:

ProcessInput
------------

Executes one or more external programs on an interval, creating
messages from the output.  If a chain of commands is used, stdout is
piped into the next command's stdin. In the event the program returns a
non-zero exit code, ProcessInput will stop, logging the exit error.

Each command is defined with the following parameters:

- Name (string):
    Each ProcessInput *must* have a name defined for logging purposes. The
    messages will be tagged with `name`.stdout or `name`.stderr in the
    `ProcessInputName` field of the heka message.
- Command (map[uint]cmd_config):
    The command is a structure that contains the full path to the
    binary, command line arguments, optional enviroment variables and
    an optional working directory. See the `cmd_config` definition
    below.  ProcessInput expects the commands to be indexed by
    integers starting with 0.
- ticker_interval (uint):
    The number of seconds to wait between runnning `command`. 
    Defaults to 15.  A ticker_interval of 0 indicates that the command
    is run once.
- stdout (bool):
    Capture stdout from `command`.  Defaults to true.
- stderr (bool):
    Capture stderr from `command`.  Defaults to false.
- decoder (string):
    Name of the decoder instance to send messages to.  Default is to inject
    messages back into the main heka router.
- parser_type (string):
    - token - splits the log on a byte delimiter (default).
    - regexp - splits the log on a regexp delimiter.
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the log line depending on the delimiter_location configuration.
    Note: when a start delimiter is used the last line in the file will not be
    processed (since the next record defines its end) until the log is rolled.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of a log line.
    - end - the regexp delimiter occurs at the end of the log line (default).
- timeout (uint):
    Timeout in seconds before any one of the commands in the chain is
    terminated.
- trim (bool):
    Trim a single trailing newline character if one exists. Default is true.

cmd_config structure:

- bin (string):
    The full path to the binary that will be executed.
- args ([]string):
    Command line arguments to pass into the executable.
- environment ([]string):
    Used to set environment variables before `command` is run. Default is nil,
    which uses the heka process's environment.
- directory (string):
    Used to set the working directory of `Bin` Default is "", which
    uses the heka process's working directory.

.. code-block:: ini

    [ProcessInput]
    name = "DemoProcessInput"
    ticker_interval = 2
    parser_type = "token"
    delimiter = " "
    stdout = true
    stderr = false
    trim = true

    [ProcessInput.Command.0]
    bin = "/bin/cat"
    Args = ["../testsupport/process_input_pipes_test.txt"]

    [ProcessInput.Command.1]
    bin = "/usr/bin/grep"
    Args = ["ignore"]


.. _config_http_input:

HttpInput
---------

Starts a HTTP client which intermittently polls a URL for data.
The entire response body is parsed by a decoder into a pipeline pack.
Data is always fetched using HTTP GET and any errors are logged and
are not fatal for the plugin.

Parameters:

- url (string):
    A HTTP URL which this plugin will regularly poll for data.
    No default URL is specified.
- ticker_interval (uint):
    Time interval (in seconds) between attempts to poll for new data.
    Defaults to 10.
- decoder (string):
    The name of the decoder used to transform the response body text into
    a structured hekad message. No default decoder is specified.

Example:

.. code-block:: ini

    [HttpInput]
    url = "http://localhost:9876/"
    ticker_interval = 5
    decoder = "ProtobufDecoder"

.. end-inputs

.. start-decoders

Decoders
========

.. _config_protobuf_decoder:

ProtobufDecoder
---------------

The ProtobufDecoder is used for Heka message objects that have been serialized
into protocol buffers format. This is the format that Heka uses to communicate
with other Heka instances, so it is almost always a good idea to include one in
your Heka configuration. The ProtobufDecoder has no configuration options.

The hekad protocol buffers message schema in defined in the `message.proto`
file in the `message` package.

Example:

.. code-block:: ini

    [ProtobufDecoder]

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_

.. _config_payloadregex_decoder:

PayloadRegexDecoder
-------------------

Decoder plugin that accepts messages of a specified form and generates new
outgoing messages from extracted data, effectively transforming one message
format into another. Can be combined w/ `message_matcher` capture groups (see
:ref:`matcher_capture_groups`) to extract unstructured information from
message payloads and use it to populate `Message` struct attributes and fields
in a more structured manner.

Parameters:

- match_regex:
    Regular expression that must match for the decoder to process the message.
- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in a regex
    in the message_matcher, and any other field that exists in the message. In
    the event that a captured name overlaps with a message field, the captured
    name's value will be used. Optional representation metadata can be added at
    the end of the field name using a pipe delimiter i.e. ResponseSize|B  =
    "%ResponseSize%" will create Fields[ResponseSize] representing the number of
    bytes.  Adding a representation string to a standard message header name
    will cause it to be added as a user defined field i.e., Payload|json will
    create Fields[Payload] with a json representation.

    Interpolated values should be surrounded with `%` signs, for example::

        [my_decoder.message_fields]
        Type = "%Type%Decoded"

    This will result in the new message's Type being set to the old messages
    Type with `Decoded` appended.
- timestamp_layout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documetation <http://golang.org/pkg/time/#pkg-
    constants>`_.
- timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`.

Example (Parsing Apache Combined Log Format):

.. code-block:: ini

    [apache_transform_decoder]
    type = "PayloadRegexDecoder"
    match_regex = '/^(?P<RemoteIP>\S+) \S+ \S+ \[(?P<Timestamp>[^\]]+)\] "(?P<Method>[A-Z]+) (?P<Url>[^\s]+)[^"]*" (?P<StatusCode>\d+) (?P<RequestSize>\d+) "(?P<Referer>[^"]*)" "(?P<Browser>[^"]*)"/'
    timestamplayout = "02/Jan/2006:15:04:05 -0700"

    [apache_transform_decoder.severity_map]
    DEBUG = 1
    WARNING = 2
    INFO = 3

    [apache_transform_decoder.message_fields]
    Type = "ApacheLogfile"
    Logger = "apache"
    Url|uri = "%Url%"
    Method = "%Method%"
    Status = "%Status%"
    RequestSize|B = "%RequestSize%"
    Referer = "%Referer%"
    Browser = "%Browser%"

.. _config_payloadjson_decoder:

PayloadJsonDecoder
------------------

This decoder plugin accepts JSON blobs and allows you to map parts
of the JSON into Field attributes of the pipelinepack message using
JSONPath syntax.

Parameters:

- json_map:
    A subsection defining a capture name that maps to a JSONPath expression.
    Each expression can fetch a single value, if the expression does
    not resolve to a valid node in the JSON message, the capture group
    will be assigned an empty string value.
- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in a JSONPath
    in the message_matcher, and any other field that exists in the message. In
    the event that a captured name overlaps with a message field, the captured
    name's value will be used. Optional representation metadata can be added at
    the end of the field name using a pipe delimiter i.e. ResponseSize|B  =
    "%ResponseSize%" will create Fields[ResponseSize] representing the number of
    bytes.  Adding a representation string to a standard message header name
    will cause it to be added as a user defined field i.e., Payload|json will
    create Fields[Payload] with a json representation.

    Interpolated values should be surrounded with `%` signs, for example::

        [my_decoder.message_fields]
        Type = "%Type%Decoded"

    This will result in the new message's Type being set to the old messages
    Type with `Decoded` appended.
- timestamp_layout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documetation <http://golang.org/pkg/time/#pkg-
    constants>`_.  The default layout is ISO8601 - the same as
    Javascript.

- timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`.

Example:

.. code-block:: ini

    [myjson_decoder]
    type = "PayloadJsonDecoder"

    [myjson_decoder.json_map]
    Count = "$.statsd.count"
    Name = "$.statsd.name"
    Pid = "$.pid"
    Timestamp = "$.timestamp"

    [myjson_decoder.severity_map]
    DEBUG = 1
    WARNING = 2
    INFO = 3

    [myjson_decoder.message_fields]
    Pid = "%Pid%"
    StatCount = "%Count%"
    StatName =  "%Name%"
    Timestamp = "%Timestamp%"

PayloadJsonDecoder's json_map config subsection  only supports a small
subset of valid JSONPath expressions.

========     =========================================
JSONPath     Description
========     =========================================
$            the root object/element
.            child operator
[]           subscript operator to iterate over arrays
========     =========================================

Examples:
---------

.. code-block:: javascript

    var s = {
        "foo": {
            "bar": [
                {
                    "baz": "こんにちわ世界",
                    "noo": "aaa"
                },
                {
                    "maz": "123",
                    "moo": 256
                }
            ],
            "boo": {
                "bag": true,
                "bug": false
            }
        }
    }

    # Valid paths
    $.foo.bar[0].baz
    $.foo.bar

PayloadXmlDecoder
-----------------

This decoder plugin accepts XML blobs in the message payload and
allows you to map parts of the XML into Field attributes of the
pipelinepack message using XPath syntax using the `xmlpath
<http://launchpad.net/xmlpath>`_ library.

Parameters:

- xpath_map:
    A subsection defining a capture name that maps to an XPath expression.
    Each expression can fetch a single value, if the expression does
    not resolve to a valid node in the XML blob, the capture group
    will be assigned an empty string value.
- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in an XPath
    in the message_matcher, and any other field that exists in the message. In
    the event that a captured name overlaps with a message field, the captured
    name's value will be used. Optional representation metadata can be added at
    the end of the field name using a pipe delimiter i.e. ResponseSize|B  =
    "%ResponseSize%" will create Fields[ResponseSize] representing the number of
    bytes.  Adding a representation string to a standard message header name
    will cause it to be added as a user defined field i.e., Payload|json will
    create Fields[Payload] with a json representation.

    Interpolated values should be surrounded with `%` signs, for example::

        [my_decoder.message_fields]
        Type = "%Type%Decoded"

    This will result in the new message's Type being set to the old messages
    Type with `Decoded` appended.
- timestamp_layout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documetation <http://golang.org/pkg/time/#pkg-
    constants>`_.  The default layout is ISO8601 - the same as
    Javascript.

- timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`.

Example:

.. code-block:: ini

    [myxml_decoder]
    type = "PayloadXmlDecoder"

    [myxml_decoder.xpath_map]
    Count = "/some/path/count"
    Name = "/some/path/name"
    Pid = "//pid"
    Timestamp = "//timestamp"

    [myxml_decoder.severity_map]
    DEBUG = 1
    WARNING = 2
    INFO = 3

    [myxml_decoder.message_fields]
    Pid = "%Pid%"
    StatCount = "%Count%"
    StatName =  "%Name%"
    Timestamp = "%Timestamp%"

PayloadXmlDecoder's xpath_map config subsection supports XPath as
implemented by the `xmlpath <http://launchpad.net/xmlpath>`_ library.

    * All axes are supported ("child", "following-sibling", etc)
    * All abbreviated forms are supported (".", "//", etc)
    * All node types except for namespace are supported
    * Predicates are restricted to [N], [path], and [path=literal] forms
    * Only a single predicate is supported per path step
    * Richer expressions and namespaces are not supported

.. _config_statstofieldsdecoder:

.. versionadded:: 0.4

StatsToFieldsDecoder
--------------------

The StatsToFieldsDecoder will parse statsd data in the `graphite message
format <http://graphite.wikidot.com/getting-your-data-into-graphite#toc4>`_
and encode the data into the message fields, in the same format produced by a
:ref:`config_stat_accum_input` plugin with the `emit_in_fields` value set to
true. This is useful if you have externally generated statsd string data
flowing through Heka that you'd like to process without having to roll your
own string parsing code.

This decoder has no configuration options, it simply expects to be passed a
message with statsd string data in the payload. Incorrect or malformed content
will cause a decoding error, dropping the message.

The fields format only contains a single "timestamp" field, so any payloads
containing multiple timestamps will end up generating a separate message for
each timestamp. Extra messages will be a copy of the original message except
a) the payload will be empty and b) the unique timestamp and related stats
will be the only message fields.

.. _config_multidecoder:

MultiDecoder
------------

This decoder plugin allows you to specify an ordered list of delegate
decoders.  The MultiDecoder will pass the PipelinePack to be decoded to each
of the delegate decoders in turn until decode succeeds.  In the case of
failure to decode, MultiDecoder will return an error and recycle the message.

Parameters:

- subs:
    A subsection is used to declare the TOML configuration for any delegate
    decoders. The default is that no delegate decoders are defined.

- order (list of strings):
    PipelinePack objects will be passed in order to each decoder in this list.
    Default is an empty list.

- name (string):
    Defaults to MultiDecoder-<address of multidecoder>.

- log_sub_errors (bool):
    If true, the DecoderRunner will log the errors returned whenever a
    delegate decoder fails to decode a message. Defaults to false.

- cascade_strategy (string):
    Specifies behavior the MultiDecoder should exhibit with regard to
    cascading through the listed decoders. Supports only two valid values:
    "first-wins" and "all". With "first-wins", each decoder will be tried in
    turn until there is a successful decoding, after which decoding will be
    stopped. With "all", all listed decoders will be applied whether or not
    they succeed. In each case, decoding will only be considered to have
    failed if *none* of the sub-decoders succeed.

Example (Two PayloadRegexDecoder delegates):

.. code-block:: ini

        [syncdecoder]
        type = "MultiDecoder"
        order = ['syncformat', 'syncraw']

        [syncdecoder.subs.syncformat]
        type = "PayloadRegexDecoder"
        match_regex = '^(?P<RemoteIP>\S+) \S+ (?P<User>\S+) \[(?P<Timestamp>[^\]]+)\] "(?P<Method>[A-Z]+) (?P<Url>[^\s]+)[^"]*" (?P<StatusCode>\d+) (?P<RequestSize>\d+) "(?P<Referer>[^"]*)" "(?P<Browser>[^"]*)" ".*" ".*" node_s:\d+\.\d+ req_s:(?P<ResponseTime>\d+\.\d+) retries:\d+ req_b:(?P<ResponseSize>\d+)'
        timestamp_layout = "02/Jan/2006:15:04:05 -0700"

        [syncdecoder.subs.syncformat.message_fields]
        RemoteIP|ipv4 = "%RemoteIP%"
        User = "%User%"
        Method = "%Method%"
        Url|uri = "%Url%"
        StatusCode = "%StatusCode%"
        RequestSize|B= "%RequestSize%"
        Referer = "%Referer%"
        Browser = "%Browser%"
        ResponseTime|s = "%ResponseTime%"
        ResponseSize|B = "%ResponseSize%"
        Payload = ""

        [syncdecoder.subs.syncraw]
        type = "PayloadRegexDecoder"
        match_regex = '^(?P<TheData>.*)'

        [syncdecoder.subs.syncraw.message_fields]
        Somedata = "%TheData%"

.. _config_sandboxdecoder:

Sandbox Decoder
---------------

The sandbox decoder provides an isolated execution environment for data parsing
and complex transformations without the need to recompile Heka.

:ref:`sandboxdecoder_settings`

.. end-decoders

.. _config_common_parameters:

Common Filter / Output Parameters
=================================

There are some configuration options that are universally available to all
Heka filter and output plugins. These will be consumed by Heka itself when
Heka initializes the plugin and do not need to be handled by the plugin-
specific initialization code.

- message_matcher (string, optional):
    Boolean expression, when evaluated to true passes the message to the filter
    for processing. Defaults to matching nothing. See: :ref:`message_matcher`
- message_signer (string, optional):
    The name of the message signer.  If  specified only messages with this
    signer  are passed to the filter for processing.
- ticker_interval (uint, optional):
    Frequency (in seconds) that a timer event will be sent to the filter.
    Defaults to not sending timer events.

.. start-filters

Filters
=======

.. _config_counter_filter:

CounterFilter
-------------

Once a second a `CounterFilter` will generate a message of type `heka.counter-
output`. The payload will contain text indicating the number of messages that
matched the filter's `message_matcher` value during that second (i.e. it
counts the messages the plugin received). Every ten seconds an extra message
(also of type `heka.counter-output`) goes out, containing an aggregate count
and average per second throughput of messages received.

Parameters: **None**

Example:

.. code-block:: ini

    [CounterFilter]
    message_matcher = "Type != 'heka.counter-output'"

.. _config_stat_filter:

StatFilter
----------

Filter plugin that accepts messages of a specfied form and uses extracted
message data to generate statsd-style numerical metrics in the form of `Stat`
objects that can be consumed by a `StatAccumulator`.

Parameters:

- Metric:
    Subsection defining a single metric to be generated

    - type (string):
        Metric type, supports "Counter", "Timer", "Gauge".
    - name (string):
        Metric name, must be unique.
    - value (string):
        Expression representing the (possibly dynamic) value that the
        `StatFilter` should emit for each received message.

- stat_accum_name (string):
    Name of a StatAccumInput instance that this StatFilter will use as its
    StatAccumulator for submitting generate stat values. Defaults to
    "StatAccumInput".

Example (Assuming you had TransformFilter inserting messages as above):

.. code-block:: ini

    [StatsdInput]
    address = "127.0.0.1:29301"
    stat_accum_name = "my_stat_accum"

    [my_stat_accum]
    flushInterval = 5

    [Hits]
    type = "StatFilter"
    stat_accum_name = "my_stat_accum"
    message_matcher = 'Type == "ApacheLogfile"'

    [Hits.Metric.bandwidth]
    type = "Counter"
    name = "httpd.bytes.%Hostname%"
    value = "%Bytes%"

    [Hits.Metric.method_counts]
    type = "Counter"
    name = "httpd.hits.%Method%.%Hostname%"
    value = "1"

.. note::

    StatFilter requires an available StatAccumulator to be running.

.. _config_sandbox_filter:

SandboxFilter
-------------
The sandbox filter provides an isolated execution environment for data analysis.

:ref:`sandboxfilter_settings`

.. _config_sandbox_manager_filter:

SandboxManagerFilter
--------------------
The sandbox manager provides dynamic control (start/stop) of sandbox filters in
a secure manner without stopping the Heka daemon.

:ref:`sandboxmanagerfilter_settings`

.. end-filters

.. start-outputs

Outputs
=======

.. _config_amqp_output:

AMQPOutput
----------

Connects to a remote AMQP broker (RabbitMQ) and sends messages to the
specified queue. The message is serialized if specified, otherwise only
the raw payload of the message will be sent. As AMQP is dynamically
programmable, the broker topology needs to be specified.

Parameters:

- URL (string):
    An AMQP connection string formatted per the `RabbitMQ URI Spec
    <http://www.rabbitmq.com/uri-spec.html>`_.
- Exchange (string):
    AMQP exchange name
- ExchangeType (string):
    AMQP exchange type (`fanout`, `direct`, `topic`, or `headers`).
- ExchangeDurability (bool):
    Whether the exchange should be configured as a durable exchange. Defaults
    to non-durable.
- ExchangeAutoDelete (bool):
    Whether the exchange is deleted when all queues have finished and there
    is no publishing. Defaults to auto-delete.
- RoutingKey (string):
    The message routing key used to bind the queue to the exchange. Defaults
    to empty string.
- Persistent (bool):
    Whether published messages should be marked as persistent or transient.
    Defaults to non-persistent.
- Serialize (bool):
    Whether published messages should be fully serialized. If set to true
    then messages will be encoded to Protocol Buffers and have the AMQP
    message Content-Type set to `application/hekad`. Defaults to true.

Example (that sends log lines from the logger):

.. code-block:: ini

    [AMQPOutput]
    url = "amqp://guest:guest@rabbitmq/"
    exchange = "testout"
    exchangeType = "fanout"
    message_matcher = 'Logger == "/var/log/system.log"'


.. _config_log_output:

LogOutput
---------

Logs messages to stdout using Go's `log` package.

Parameters:

- payload_only (bool, optional):
    If set to true, then only the message payload string will be output,
    otherwise the entire `Message` struct will be output in JSON format.

Example:

.. code-block:: ini

    [counter_output]
    type = "LogOutput"
    message_matcher = "Type == 'heka.counter-output'"
    payload_only = true

.. _config_file_output:

FileOutput
----------

Writes message data out to a file system.

Parameters:

- path (string):
    Full path to the output file.
- format (string, optional):
    Output format for the message to be written. Supports `json` or
    `protobufstream`, both of which will serialize the entire `Message`
    struct, or `text`, which will output just the payload string. Defaults to
    ``text``.
- prefix_ts (bool, optional):
    Whether a timestamp should be prefixed to each message line in the file.
    Defaults to ``false``.
- perm (string, optional):
    File permission for writing. A string of the octal digit representation.
    Defaults to "644".

Example:

.. code-block:: ini

    [counter_file]
    type = "FileOutput"
    message_matcher = "Type == 'heka.counter-output'"
    path = "/var/log/heka/counter-output.log"
    prefix_ts = true
    perm = "666"

.. _config_tcp_output:

TcpOutput
---------

Output plugin that serializes messages into the Heka protocol format and
delivers them to a listening TCP connection. Can be used to deliver messages
from a local running Heka agent to a remote Heka instance set up as an
aggregator and/or router.

Parameters:

- address (string):
    An IP address:port to which we will send our output data.

Example:

.. code-block:: ini

    [aggregator_output]
    type = "TcpOutput"
    address = "heka-aggregator.mydomain.com:55"
    message_matcher = "Type != 'logfile' && Type != 'heka.counter-output' && Type != 'heka.all-report'"

.. _config_dashboard_output:

DashboardOutput
---------------

Specialized output plugin that listens for certain Heka reporting message
types and generates JSON data which is made available via HTTP for use in web
based dashboards and health reports.

Parameters:

- ticker_interval (uint):
    Specifies how often, in seconds, the dashboard files should be updated.
    Defaults to 5.
- message_matcher (string):
    Defaults to `"Type == 'heka.all-report' || Type == 'heka.sandbox-output'
    || Type == 'heka.sandbox-terminated'"`. Not recommended to change this
    unless you know what you're doing.
- address (string):
    An IP address:port on which we will serve output via HTTP. Defaults to
    "0.0.0.0:4352".
- working_directory (string):
    File system directory into which the plugin will write data files and from
    which it will serve HTTP. The Heka process must have read / write access
    to this directory. Relative paths will be evaluated relative to the Heka
    base directory. Defaults to "dashboard" (i.e. "$(BASE_DIR)/dashboard").
- static_directory (string):
    File system directory where the Heka dashboard source code can be found.
    The Heka process must have read access to this directory. Relative paths
    will be evaluated relative to the Heka base directory. Defaults to
    "/usr/share/heka/dasher".

Example:

.. code-block:: ini

    [DashboardOutput]
    ticker_interval = 30

.. _config_elasticsearch_output:

ElasticSearchOutput
-------------------

Output plugin that serializes messages into JSON structures and uses HTTP requests
to insert them into an ElasticSearch database.

Parameters:

- cluster (string):
    ElasticSearch cluster name. Defaults to "elasticsearch"
- index (string):
    Name of the ES index into which the messages will be inserted.
    If Field Name|Type|Hostname|Pid|UUID|Logger|EnvVersion|Severity
    are placed between within a %{}, it will be interpolated to their message value.
    Defaults to "heka-%{2006.01.02}".
- type_name (string):
    Name of ES record type to create. Defaults to "message".
    If Field Name|Type|Hostname|Pid|UUID|Logger|EnvVersion|Severity
    are placed between within a %{}, it will be interpolated to their message value.
- flush_interval (int):
    Interval at which accumulated messages should be bulk indexed into
    ElasticSearch, in milliseconds. Defaults to 1000 (i.e. one second).
- flush_count (int):
    Number of messages that, if processed, will trigger them to be bulk
    indexed into ElasticSearch. Defaults to 10.
- format (string):
    Message serialization format, either "clean", "logstash_v0", "payload" or
    "raw". "clean" is a more concise JSON representation of the message,
    "logstash_v0" outputs in a format similar to Logstash's original (i.e.
    "version 0") ElasticSearch schema, "payload" passes the message payload
    directly into ElasticSearch, and "raw" is a full JSON representation of
    the message. Defaults to "clean".
- fields ([]string):
    If the format is "clean", then the 'fields' parameter can be used to
    specify that only specific message data should be indexed into
    ElasticSearch. Available fields to choose are "Uuid", "Timestamp", "Type",
    "Logger", "Severity", "Payload", "EnvVersion", "Pid", "Hostname", and
    "Fields" (where "Fields" causes the inclusion of any and all dynamically
    specified message fields. Defaults to all.
- timestamp (string):
    Format to use for timestamps in generated ES documents. Defaults to
    "2006-01-02T15:04:05.000Z".
- server (string):
    ElasticSearch server URL. Supports http://, https:// and udp:// urls.
    Defaults to "http://localhost:9200".
- ESIndexFromTimestamp (bool):
    When generating the index name use the timestamp from the message
    instead of the current time. Defaults to false.

Example:

.. code-block:: ini

    [ElasticSearchOutput]
    message_matcher = "Type == 'sync.log'"
    cluster = "elasticsearch-cluster"
    index = "synclog-%{2006.01.02.15.04.05}"
    type_name = "sync.log.line"
    server = "http://es-server:9200"
    format = "clean"
    flush_interval = 5000
    flush_count = 10

.. _config_whisper_output:

WhisperOutput
-------------

WhisperOutput plugins parse the "statmetric" messages generated by a
StatAccumulator and write the extracted counter, timer, and gauge data out to
a `graphite <http://graphite.wikidot.com/>`_ compatible `whisper database
<http://graphite.wikidot.com/whisper>`_ file tree structure.

Parameters:

- base_path (string):
    Path to the base directory where the whisper file tree will be written.
    Absolute paths will be honored, relative paths will be calculated relative
    to the Heka base directory. Defaults to "whisper" (i.e.
    "$(BASE_DIR)/whisper").
- default_agg_method (int):
    Default aggregation method to use for each whisper output file. Supports
    the following values:

    0. Unknown aggregation method.
    1. Aggregate using averaging. (default)
    2. Aggregate using summation.
    3. Aggregate using last received value.
    4. Aggregate using maximum value.
    5. Aggregate using minimum value.
- default_archive_info ([][]int):
    Default specification for new whisper db archives. Should be a sequence of
    3-tuples, where each tuple describes a time interval's storage policy:
    [<offset> <# of secs per datapoint> <# of datapoints>] (see `whisper docs
    <graphite.readthedocs.org/en/latest/whisper.html>`_ for more info). Defaults
    to:

    .. code-block:: ini

        [ [0, 60, 1440], [0, 900, 8], [0, 3600, 168], [0, 43200, 1456]]

    The above defines four archive sections. The first uses 60 seconds for
    each of 1440 data points, which equals one day of retention. The second
    uses 15 minutes for each of 8 data points, for two hours of retention. The
    third uses one hour for each of 168 data points, or 7 days of retention.
    Finally, the fourth uses 12 hours for each of 1456 data points,
    representing two years of data.
- folder_perm (string):
    Permission mask to be applied to folders created in the whisper database
    file tree. Must be a string representation of an octal integer. Defaults
    to "700".

Example:

.. code-block:: ini

    [WhisperOutput]
    message_matcher = "Type == 'heka.statmetric'"
    default_agg_method = 3
    default_archive_info = [ [0, 30, 1440], [0, 900, 192], [0, 3600, 168], [0, 43200, 1456] ]
    folder_perm = "755"

.. _config_nagios_output:

NagiosOutput
---------------

Specialized output plugin that listens for Nagios external command message types
and generates an HTTP request against the Nagios cmd.cgi API. Currently the
output will only send passive service check results.  The message payload must
consist of a state followed by a colon and then the message i.e.,
"OK:Service is functioning properly". The valid states are:
OK|WARNING|CRITICAL|UNKNOWN.  Nagios must be configured with a service name that
matches the Heka plugin instance name and the hostname where the plugin is
running.

Parameters:

- url (string, optional):
    An HTTP URL to the Nagios cmd.cgi. Defaults to "http://localhost/nagios/cgi-bin/cmd.cgi".
- username (string, optional):
    Username used to authenticate with the Nagios web interface. Defaults to "".
- password (string, optional):
    Password used to authenticate with the Nagios web interface. Defaults to "".
- responseheadertimeout (uint, optional):
    Specifies the amount of time, in seconds, to wait for a server's response
    headers after fully writing the request. Defaults to 2.

Example configuration to output alerts from SandboxFilter plugins:

.. code-block:: ini

    [NagiosOutput]
    url = "http://localhost/nagios/cgi-bin/cmd.cgi"
    username = "nagiosadmin"
    password = "nagiospw"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'nagios-external-command' && Fields[payload_name] == 'PROCESS_SERVICE_CHECK_RESULT'"

Example Lua code to generate a Nagios alert:

.. code-block:: lua

    output("OK:Alerts are working!")
    inject_message("nagios-external-command", "PROCESS_SERVICE_CHECK_RESULT")

.. _config_carbon_output:

CarbonOutput
------------

CarbonOutput plugins parse the "stat metric" messages generated by a
StatAccumulator and write the extracted counter, timer, and gauge data out to
a `graphite <http://graphite.wikidot.com/>`_ compatible `carbon
<http://graphite.wikidot.com/carbon>`_ daemon.  Output is written over
a TCP socket using the `plaintext <http://graphite.readthedocs.org/en/1.0/feeding-carbon.html#the-plaintext-protocol>`_ protocol.

Parameters:

- address (string):
    An IP address:port on which this plugin will write to.
    Defaults to: localhost:2003

Example:

.. code-block:: ini

    [CarbonOutput]
    message_matcher = "Type == 'heka.statmetric'"
    address = "localhost:2003"


.. end-outputs

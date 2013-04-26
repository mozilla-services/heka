.. _configuration:

=================
Configuring hekad
=================

.. start-hekad-config

A hekad configuration file specifies what inputs, decoders, filters, and
outputs will be loaded. The configuration file is in `TOML
<https://github.com/mojombo/toml>`_ format. TOML looks is very similar to INI
configuration formats, but with slightly more rich data structures and nesting
support.

The config file is broken into sections, with each section representing a
single instance of a plugin. The section name specifies the name of the
plugin, and the "type" parameter specifies the plugin type; this must match
one of the types registered via the `pipeline.RegisterPlugin` function. For
example, the following section describes a plugin named "tcp:5565", an
instance of Heka's plugin type "TcpInput":

.. code-block:: ini

    [tcp:5565]
    type = "TcpInput"
    address = ":5565"

If you choose a plugin name that also happens to be a plugin type name, then
you can omit the "type" parameter from the section and the specified name will
be used as the type. Thus, the following section describes a plugin named
"TcpInput", also of type "TcpInput":

.. code-block:: ini

    [TcpInput]
    address = ":5566"

Note that it's fine to have more than one instance of the same plugin type, as
long as their configurations don't interfere with each other.

Any values other than "type" in a section, such as "address" in the above
examples, will be passed through to the plugin for internal configuration (see
:ref:`plugin_config`).

A JsonDecoder and ProtobufDecoder will be automatically setup if not specified
explicitly in the configuration file.

.. end-hekad-config

Example hekad.toml File
=======================

.. start-hekad-toml

.. code-block:: ini

    # Listens for Heka protocol on TCP port 5565.
    [TcpInput]
    address = ":5565"

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

Command Line Options
====================

.. start-options

``-version``
    Output the version number, then exit.

``-config`` `config_file`
    Specify the configuration file to use; the default is /etc/hekad.json.  (See hekad.config(5).)

``-cpuprof`` `output_file`
    Turn on CPU profiling of hekad; output is logged to the `output_file`.

``-maxprocs`` `int`
    Enable multi-core usage; the default is 1 core. More cores will generally
    increase message throughput. Best performance is usually attained by
    setting this to 2 x (number of cores). This assumes each core is
    hyper-threaded.

``-memprof`` `output_file`
    Enable memory profiling; output is logged to the `output_file`.

``-poolsize`` `int`
    Specify the pool size of maximum messages that can exist; default is 100
    which is usually sufficient and of optimal performance.

``-decoder_poolsize`` `int`
    Specify the number of decoder sets to spin up for use converting input
    data to Heka's Message objects. Default is 4, optimal value is variable,
    depending on number of total running plugins, number of expected
    concurrent connections, amount of expected traffic, and number of
    available cores on the host.

``-plugin_chansize`` `int`
    Specify the buffer size for the input channel for the various Heka
    plugins. Defaults to 50, which is usually sufficient and of optimal
    performance.

.. end-options

.. start-inputs

Inputs
======

.. _config_udp_input:

UdpInput
--------

Listens on a specific UDP address and port for messages. If the message is
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

Example:

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"

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

Example:

.. code-block:: ini

    [TcpInput]
    address = ":5565"

    [TcpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [TcpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [TcpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"

.. _config_statsd_input:

StatsdInput
-----------

Exposes internal `StatMonitor` API into which other Heka plugins can insert
numeric statistics, and optionally listens for `statsd protocol
<https://github.com/b/statsd_spec>`_ `counter`, `timer`, or `gauge` messages
on a UDP port. Generates Heka messages of type `statmetric`, with a string
payload in the format that is accepted by the `carbon
<http://graphite.wikidot.com/carbon>`_ portion of `graphite
<http://graphite.wikidot.com/>`_.

Parameters:

- address (string, optional):
    An IP address:port on which this plugin will expose a statsd server.
- flushinterval (int):
    Time interval (in seconds) between generated `statmetric` messages.
    Defaults to 10.
- percentthreshold (int):
    Percent threshold to use for computing "upper_N%" type stat values.
    Defaults to 90.

Example:

.. code-block:: ini

    [StatsdInput]
    address = ":8125"
    flushinterval = 5

.. end-inputs

.. start-decoders

Decoders
========

A decoder may be specified for each encoding type defined in message.pb.go.
Unless you are using a custom decoder you probably won't need to specify these
by hand, by default the JsonDecoder and ProtobufDecoder will be configured as
if you had included the following configuration.

Example:

.. code-block:: ini

    [JsonDecoder]
    encoding_name = "JSON"

    [ProtobufDecoder]
    encoding_name = "PROTOCOL_BUFFER"

The JsonDecoder converts JSON serialized Heka messages to `Message` struct
objects. The `encoding_name` setting means that this decoder should be used
for any Heka protocol messages that have the encoding header of JSON. The
ProtobufDecoder converts protocol buffers serialized messages to `Message`
struct objects. The hekad protocol buffers message schema in defined in the
`message.proto` file in the `message` package.

.. note::

    These sections remain configurable explicitly in the configuration
    file for possible future use where a different Decoder may want to
    handle one of these encodings.

.. seealso:: `Protocol Buffers - Google's data interchange format
   <http://code.google.com/p/protobuf/>`_

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

.. _config_transform_filter:

TransformFilter
---------------

Heka filter plugin that accepts messages of a specified form and generates new
outgoing messages from extracted data, effectively transforming one message
format into another. Can be combined w/ `message_matcher` capture groups (see
:ref:`matcher_capture_groups`) to extract unstructured information from
message payloads and use it to populate `Message` struct attributes and fields
in a more structured manner.

Parameters:

- SeverityMap:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.

- MessageFields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in a regex
    in the message_matcher, and any other field that exists in the message. In
    the event that a captured name overlaps with a message field, the captured
    name's value will be used.

    Interpolated values should be surrounded with `%` signs, for example::

        [my_filter.MessageFields]
        Type = "%Type%Transformed"

    This will result in the new message's Type being set to the old messages
    Type with `Transformed` appended.

- TimestampLayout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documetation <http://golang.org/pkg/time/#pkg-constants>`_.

Example (Parsing Apache Combined Log Format):

.. code-block:: ini

    [apache_transform_filter]
    type = "TransformFilter"
    message_matcher = 'Type == "logfile" && Logger == "/var/log/httpd.log" && Payload ~= /^(?P<RemoteIP>\S+) \S+ \S+ \[(?P<Timestamp>[^\]]+)\] "(?P<Method>[A-Z]+) (?P<Url>[^\s]+)[^"]*" (?P<StatusCode>\d+) (?P<Bytes>\d+) "(?P<Referer>[^"]*)" "(?P<Browser>[^"]*)"/'
    timestamplayout = "02/Jan/2006:15:04:05 +0100"

    [apache_transform_filter.SeverityMap]
    DEBUG = 1
    WARNING = 2
    INFO = 3

    [apache_transform_filter.MessageFields]
    Type = "ApacheLogfile"
    Logger = "apache"
    Url = "%Url%"
    Method = "%Method%"
    Status = "%Status%"
    Bytes = "%Bytes%"
    Referer = "%Referer%"
    Browser = "%Browser%"


.. _config_stat_filter:

StatFilter
----------

Filter plugin that accepts messages of a specfied form and uses extracted
message data to generate statsd-style numerical metrics. Can be combined w/
`message_matcher` capture groups (see :ref:`matcher_capture_groups`) to parse
message payloads and generate counter and timer data from extracted content.

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

- StatsdInputName (string, optional):
    Configured `name` value for a running `StatsdInput` plugin into which
    stats can be fed. Defaults to `StatsdInput`.

Example (Assuming you had TransformFilter inserting messages as above):

.. code-block:: ini

    [StatsdInput]
    address = "127.0.0.1:29301"
    flushInterval = 5

    [Hits]
    type = "StatFilter"
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

    StatFilter requires the StatsdInput to be running.

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
- perm (int, optional):
    File permission for writing. Defaults to ``0666``.

Example:

.. code-block:: ini

    [counter_file]
    type = "FileOutput"
    message_matcher = "Type == 'heka.counter-output'"
    path = "/var/log/heka/counter-output.log"
    prefix_ts = true

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
- address (string, optional):
    An IP address:port on which we will serve output via HTTP. Defaults to
    "0.0.0.0:4352".
- workingdirectory (string, optional):
    File system directory into which the plugin will write data files and from
    which it will serve HTTP. The Heka process must have read / write access
    to this directory. Defaults to "./dashboard".

Example:

.. code-block:: ini

    [DashboardOutput]
    ticker_interval = 60
    message_matcher = "Type == 'heka.all-report' || Type == 'heka.sandbox-output' || Type == 'heka.sandbox-terminated'"

.. _config_whisper_output:

WhisperOutput
-------------

WhisperOutput plugins parse `statmetric` message types and write the extracted
counter, timer, and gauge data out to a `graphite
<http://graphite.wikidot.com/>`_ compatible `whisper database
<http://graphite.wikidot.com/whisper>`_ file tree structure.

Parameters:

- basepath (string, optional):
    Path to the base directory where the whisper file tree will be written. Defaults
    to "/var/run/hekad/whisper".
- defaultaggmethod (int, optional):
    Default aggregation method to use for each whisper output file. Supports
    the following values:

    0. Unknown aggregation method.
    1. Aggregate using averaging. (default)
    2. Aggregate using summation.
    3. Aggregate using last received value.
    4. Aggregate using maximum value.
    5. Aggregate using minimum value.
- defaultarchiveinfo ([][]int, optional):
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

Example:

.. code-block:: ini

    [WhisperOutput]
    message_matcher = "Type == 'statmetric'"
    defaultaggmethod = 3
    defaultarchiveinfo = [ [0, 30, 1440], [0, 900, 192], [0, 3600, 168], [0, 43200, 1456] ]

.. end-outputs

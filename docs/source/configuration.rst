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
    address = "127.0.0.1:5565"

If you choose a plugin name that also happens to be a plugin type name, then
you can omit the "type" parameter from the section and the specified name will
be used as the type. Thus, the following section describes a plugin named
"TcpInput", also of type "TcpInput":

.. code-block:: ini

    [TcpInput]
    address = "127.0.0.1:5566"

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
    address = "127.0.0.1:5565"

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

UdpInput
--------

Listens on a specific UDP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name
is added to the pipeline pack and can be use to accept messages using the
message_signer configuration option.

Parameters:

- address (string): An IP address:port.
- signer: Optional TOML subsection. Section name consists of a signer name,
          underscore, and numeric version of the key
    - hmac_key (string): The hash key used to sign the message.

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

TcpInput
--------

Listens on a specific TCP address and port for messages. If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name 
is added to the pipeline pack and can be use to accept messages using the 
message_signer configuration option.

Parameters:

- address (string): An IP address:port.
- signer: Optional TOML subsection. Section name consists of a signer name,
          underscore, and numeric version of the key
    - hmac_key (string): The hash key used to sign the message.

Example:

.. code-block:: ini

    [TcpInput]
    address = "127.0.0.1:5565"

    [TcpInput.signer.ops_0]
    hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
    [TcpInput.signer.ops_1]
    hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

    [TcpInput.signer.dev_1]
    hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"

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

- address (string, optional): An IP address:port.
- flushinterval (int): Time interval (in seconds) between generated
                       `statmetric` messages. Defaults to 10.
- percentthreshold (int): Percent threshold to use for computing "upper_N%"
                          type stat values. Defaults to 90.

Example:

.. code-block:: ini

    [StatsdInput]
    address = "127.0.0.1:8125"
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

.. start-filters

Filters
=======

.. _common_filter_parameters:

Common Parameters
-----------------

- message_matcher (string): Boolean expression, when evaluated to true passes the message to the filter for processing. See: :ref:`message_matcher`
- message_signer (string - optional): The name of the message signer.  If specified only messages with this signer are passed to the filter for processing.
- ticker_interval (uint):  Frequency in seconds that a timer event will be sent to the filter


CounterFilter
----------------
Parameters: **None**

Once a second the count of every message that was matched is output and  every
ten seconds an aggregate count with an average per second is output.

SandboxFilter
-------------
The sandbox filter provides an isolated execution environment for data analysis.

:ref:`sandboxfilter_settings`

SandboxManagerFilter
--------------------
The sandbox manager provides dynamic control (start/stop) of sandbox filters in
a secure manner without stopping the Heka daemon.

:ref:`sandboxmanagerfilter_settings`

.. end-filters

.. start-outputs

Outputs
=======

FileOutput
----------

Parameters:

- Path (string): Path to the file to write.
- Format (string): Output format for the message to be written.
  Can be either `json` or `text`. Defaults to ``text``.
- Prefix_ts (bool): Whether a timestamp should be prefixed to each
  message line in the file. Defaults to ``false``.
- Perm (int): File permission for writing. Defaults to ``0666``.

Writes a message to the designated file in the format given (including
a prefixed timestamp if configured).

LogOutput
---------

Parameters: **None**

Logs the message to stdout.

.. end-outputs

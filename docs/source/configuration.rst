.. _configuration:

=================
Configuring hekad
=================

.. start-hekad-config

A hekad configuration file contains what inputs, decoders, filters, and
outputs will be loaded. The configuration file is in TOML format, which
is very similar to INI configuration formats. The config file is broken
into sections, each section configures a single instance of a plugin.

Each section must either be named for the specific plugin it is
configuring, or include the type which must designate the plugin being
configured. When a section name and the type differs, the section name
is considered the 'name' of that particular configured plugin. This is
convenient when setting up multiple instances of the same plugin that
have varying configurations.

If the name of a section doesn't directly reference a plugin, then that
section must include a `type` naming the plugin type being configured.
Additional settings for the section are passed through to the plugin as
its configuration values.

The JsonDecoder and ProtobufDecoder will be automatically setup if not
specified explicitly in the configuration file.

.. end-hekad-config

Example hekad.toml File
=======================

.. start-hekad-toml

.. code-block:: ini

    [tcp-5565]
    address = "127.0.0.1:5565"

    [debug]
    type = "LogOutput"

    [CounterFilter]
    type = "CounterFilter"
    message_matcher = "Type == 'hekabench' && EnvVersion == '0.8'"
    output_timer = 1

    [lua_sandbox]
    type = "SandboxFilter"
    message_matcher = "Type == 'hekabench' && EnvVersion == '0.8'"
    output_timer = 1

    [lua_sandbox.settings]
    type = "lua"
    preserve_data = true
    filename = "lua/sandbox.lua"
    memory_limit = 32767
    instruction_limit = 1000
    output_limit = 1024

.. end-hekad-toml

This example will accept TCP input on the specified address, decode
messages that arrive serialized as JSON or Protobuf, pass the message
to all filters that match the message, and then write the filter output
to the debug log.

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

MessageGeneratorInput
---------------------

Parameters: **None**

Allows other plug-ins to generate messages. This input plug-in makes a
channel available for other plug-ins that need to create messages at
different points in time. Plug-ins requiring this input will indicate
it as a prerequisite.

Multiple plug-ins may use a single instance of the
MessageGeneratorInput.

UdpInput
--------

Parameters:

- Address (string): An IP address:port.

Example:

.. code-block:: ini

    [UdpInput]
    address = "127.0.0.1:4880"

Listens on a specific UDP address and port for messages.

TcpInput
--------

Parameters:

- Address (string): An IP address:port.
        [TcpInput.signer.test_0]
        hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
- signer (object - optional): The TOML key name consists of a signer name, underscore, and numeric version of the key
    - hmac_key: The hash key used to sign the message.

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

Listens on a specific TCP address and port for messages.  If the message is
signed it is verified against the signer name and specified key version. If
the signature is not valid the message is discarded otherwise the signer name 
is added to the pipeline pack and can be use to accept messages using the 
message_signer configuration option.

.. end-inputs

.. start-decoders

Decoders
========

A decoder may be specified for each encoding type defined in
message.pb.go. By default the JsonDecoder and ProtobufDecoder will be
configured as if you had included this portion.

Example:

.. code-block:: ini

    [JsonDecoder]
    encoding_name = "JSON"

    [ProtobufDecoder]
    encoding_name = "PROTOCOL_BUFFER"


The JSON decoder converts JSON serialized Metlog client messages to
hekad messages.  The PROTOCOL_BUFFER decoder converts protobuf
serialized messages into hekad. The hekad message schema in defined in
message.proto.

.. note::

    These sections remain configurable explicitly in the configuration
    file for possible future use where a different Decoder may want to
    handle one of these encodings.

.. seealso:: `Protocol Buffers - Google's data interchange format <http://code.google.com/p/protobuf/>`_

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

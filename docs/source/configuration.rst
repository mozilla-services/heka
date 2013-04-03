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
    filename = "lua/sandbox.lua"
    memory_limit = 32767
    instruction_limit = 1000

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
    Toggle the pool size of maximum messages that can exist; default is 1000
    which is usually sufficient and performs optimally.

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

Example:

.. code-block:: ini

    [TcpInput]
    address = "127.0.0.1:5565"

Listens on a specific TCP address and port for messages.

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

Common Parameters:

- message_matcher (string): Boolean expression, when evaluated to true passes the message to the filter for processing
- output_timer (uint):  Frequency in seconds that a timer event will be sent to the filter
- outputs ([]string): List of output destinations for the data produced (referenced by name from the 'outputs' section)


CounterFilter
----------------
Parameters: **None**

Once a second the count of every message that was matched is output and  every
ten seconds an aggregate count with an average per second is output.

SandboxFilter
-------------
Parameters:

- settings (object): Sandbox specific settings

   - type (string): Sandbox virtual machine, currently only "lua" is supported
   - filename (string): Path to the Lua script
   - memory_limit (uint): Maximum number of bytes the sandbox is allowed to consume before being terminated
   - instruction_limit (uint): Maximum number of Lua instructions the sandbox is allowed to consume (per function call) before being terminated

Example:

.. code-block:: ini

    [lua_sandbox]
    type = "SandboxFilter"
    message_matcher = "Type == 'hekabench' && EnvVersion == '0.8'"
    output_timer = 1

    [lua_sandbox.settings]
    type = "lua"
    filename = "lua/sandbox.lua"
    memory_limit = 32767
    instruction_limit = 1000

Outputs whatever data is produced by the sandbox to the specified destinations.

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

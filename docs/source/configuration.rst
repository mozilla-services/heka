.. _configuration:

=================
Configuring hekad
=================

`hekad` is the primary daemon application deployed to collect node data
and route messages to the back-ends. It's a multi-purpose agent as it
can act in several different roles depending on how its configured.

A simple example configuration file:

.. code-block:: javascript

   {
       "inputs": [
                 {"name": "tcp:5565",
                     "type": "TcpInput",
                     "address": "127.0.0.1:5565"
                 }
                 ],
       "decoders": [
                   {"type": "JsonDecoder", "encoding_name": "JSON"},
                   {"type": "ProtobufDecoder", "encoding_name": "PROTOCOL_BUFFER"}

                   ],
       "outputs": [
                  {"name": "debug", "type": "LogOutput"}
                  ],
       "filters": {
           "counter" :
           {
               "type": "CounterFilter",
               "message_matcher": "Type == 'hekabench' && EnvVersion == '0.8'",
               "output_timer" : 1,
               "outputs" : ["debug"]
           },
           "lua_sandbox" :
           {
               "type": "SandboxFilter",
               "message_matcher": "Type == 'hekabench' && EnvVersion == '0.8'",
               "output_timer" : 1,
               "outputs" : ["debug"],
               "sandbox": {
                   "type" : "lua",
                   "filename" : "lua/sandbox.lua",
                   "memory_limit" : 32767,
                   "instruction_limit" : 1000
               }
           }
       }
   }

This example will accept TCP input on the specified address, decode messages
that arrive serialized as JSON or Protobuf, pass the message to all filters
that match the message, and then write the filter output to the debug log.

Inputs, decoders, filters, and outputs are all hekad plug-ins and have
some configuration keys in common. Individual plug-ins may have
additional optional or required parameters as well.

Each plug-in declaration results in an 'instance' of the plug-in being
created when `hekad` starts up.

Common Roles
============

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

Command Line Options
====================

.. include:: man/usage.rst
    :start-after: start-options
    :end-before: end-options

Configuration File Format
=========================

hekad's configuration file is a plain JSON text file that designates
several keys to configure the various hekad plug-ins:

- inputs
- decoders
- filters
- outputs

Inputs, decoders, filters, and output plug-ins must each be specified
by `type` and may optionally supply a `name` to be used for referring
to it later. Plug-ins may be specified multiple times as needed. For
example, if `hekad` should listen on multiple UDP sockets then it can
be added twice with the appropriate `address` for each IP/port to
listen on. They then must be named to avoid plug-in name conflicts.

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

.. code-block:: javascript

    {
        "type": "UdpInput",
        "address": "127.0.0.1:4880"
    }

Listens on a specific UDP address and port for messages.

TcpInput
--------

Parameters:

    - Address (string): An IP address:port.

Example:

.. code-block:: javascript

    {
        "name": "tcp:5565",
        "type": "TcpInput",
        "address": "127.0.0.1:5565"
    }

Listens on a specific TCP address and port for messages.

Decoders
========

A decoder should be specified for each encoding type defined in message.pb.go

.. code-block:: javascript

      {"type": "JsonDecoder", "encoding_name": "JSON"},
      {"type": "ProtobufDecoder", "encoding_name": "PROTOCOL_BUFFER"}


The JSON decoder converts JSON serialized Metlog client messages to hekad
messages.  The PROTOCOL_BUFFER decoder converts protobuf serialized messages
into hekad. The hekad message schema in defined in message.proto.

.. seealso:: `Protocol Buffers - Google's data interchange format <http://code.google.com/p/protobuf/>`_

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

    - sandbox (object): Sandbox specific configuration
           - type (string): Sandbox virtual machine, currently only "lua" is supported
           - filename (string): Path to the Lua script
           - memory_limit (uint): Maximum number of bytes the sandbox is allowed to consume before being terminated
           - instruction_limit (uint): Maximum number of Lua instructions the sandbox is allowed to consume (per function call) before being terminated

Outputs whatever data is produced by the sandbox to the specified destinations.

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

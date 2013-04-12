.. _plugins:

==============
Extending Heka
==============

The core of the Heka engine is written in the `Go <http://golang.org>`_
programming language. Heka supports four different types of plugins (inputs,
decoders, filters, and outputs), which are also written in Go. This document
will try to provide enough information for developers to extend Heka by
implementing their own custom plugins. It assumes a small amount of
familiarity with Go, although any reasonably experienced programmer will
probably be able to follow along with no trouble.

*NOTE*: Heka also supports the use of `Lua <http://www.lua.org>`_ for
dynamically loaded, security sandboxed filter plugins. This document only
covers the use of Go plugins. You can learn more about sandboxed plugins in
the :ref:`sandbox` section.

.. _extending_definitions:

Definitions
===========

You should be familiar with the :ref:`glossary` terminology before proceeding.

.. _extending_overview:

========
Overview
========

Each Heka plugin type performs a specific task: inputs receive input from the
outside world and inject the data into the Heka pipeline, decoders turn binary
data into Message objects that Heka can process, filters perform arbitrary
processing of Heka message data, and outputs send data from Heka back to the
outside world. Each specific plugin has some custom behaviour, but it also
shares behaviour w/ every other plugin of that type. A UDPInput and a TCPInput
listen on the network differently, and a LogFileInput (reading logs off the
file system) doesn't listen on the network at all, but all of these inputs
need to interact w/ the Heka system to access data structures, gain access to
decoders to which we pass our incoming data, respond to shutdown and other
system events, etc.

To support this, each Heka plugin actually consists of two parts: the plugin
itself, and an accompanying "plugin runner". Inputs have an InputRunner,
decoders have a DecoderRunner, filters have a FilterRunner, and Outputs have
an OutputRunner. The plugin itself contains the plugin-specific behaviour, and
is provided by the plugin developer. The plugin runner contains the shared (by
type) behaviour, and is provided by Heka. When Heka starts a plugin, it a)
creates and configures a plugin instance of the appropriate type, b) creates a
plugin runner instance of the appropriate type (passing in the plugin), and c)
calls the Start method of the plugin runner. Most plugin runners (excepting
decoders) then call the plugin's Run method, passing themselves and an
additional PluginHelper object in as arguments so the plugin code can use
their exposed APIs to interact w/ the Heka system.

For inputs, filters, and outputs, there's a 1:1 correspondence between
sections specified in the config file and running plugin instances. This is
not the case for decoders, however; a pool of decoder instances are created so
that messages from different sources can be decoded in parallel. Plugins can
gain access to a set of running decoders using the DecoderSet method of the
provided PluginHelper.

.. _plugin_config:

Plugin Configuration
====================

Heka uses `TOML <https://github.com/mojombo/toml>`_ as its configuration file
format (see: :ref:`configuration`), and provides a simple mechanism through
which plugins can integrate with the configuration loading system to
initialize themselves from settings in hekad's config file.

The minimal shared interface that a Heka plugin must implement in order to use
the config system is (unsurprisingly) `Plugin`, defined in `pipeline_runner.go
<https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_::

    type Plugin interface {
            Init(config interface{}) error
    }

During Heka initialization an instance of every input, filter, and output
plugin (and many instances of every decoder) listed in the configuration file
will be created. The TOML configuration for each plugin will be parsed and the
resulting configuration object will be passed in to the above specified `Init`
method. The argument is of type `interface{}`; by default the underlying type
will be `*pipeline.PluginConfig`, a map object that provides config data as
key/value pairs. There is also a way for plugins to specify a custom struct to
be used instead of the generic `PluginConfig` type (see
:ref:`custom_plugin_config`). In either case, the config object will be
already loaded with values read in from the TOML file, which your plugin code
can then use to initialize itself.

As an example, imagine we're writing a filter that will deliver messages to a
specific output plugin, but only if they come from a list of approved hosts.
Both 'hosts' and 'output' would be required in the plugin's config section.
Here's one version of what the plugin definition and `Init` method might look
like::

    type HostFilter struct {
        hosts  map[string]bool
        output string
    }

    // Extract hosts value from config and store it on the plugin instance.
    func (f *HostFilter) Init(config interface{}) error {
        var (
            hostsConf  interface{}
            hosts      []interface{}
            host       string
            outputConf interface{}
            ok         bool
        )
        conf := config.(pipeline.PluginConfig)
        if hostsConf, ok = conf["hosts"]; !ok {
            return errors.New("No 'hosts' setting specified.")
        }
        if hosts, ok = hostsConf.([]interface{}); !ok {
            return errors.New("'hosts' setting not a sequence.")
        }
        if outputConf, ok = conf["output"]; !ok {
            return errors.New("No 'output' setting specified.")
        }
        if f.output, ok = outputConf.(string); !ok {
            return errors.New("'output' setting not a string value.")
        }
        f.hosts = make(map[string]bool)
        for _, h := range hosts {
            if host, ok = h.(string); !ok {
                return errors.New("Non-string host value.")
            }
            f.hosts[host] = true
        }
        return nil
    }

(Note that this is a bit of a contrived example. In practice, you would
generally route messages to specific outputs using the
:ref:`message_matcher`.)

.. _custom_plugin_config:

Custom Plugin Config Structs
============================

In simple cases it might be fine to get plugin configuration data as a generic
map of keys and values, but if there are more than a couple of config settings
then checking for, extracting, and validating the values quickly becomes a lot
of work. Heka plugins can instead specify a schema struct for their
configuration data, into which the TOML configuration will be decoded.

Plugins that wish to provide a custom configuration struct should implement
the `HasConfigStruct` interface defined in the `config.go
<https://github.com/mozilla-services/heka/blob/dev/pipeline/config.go>`_
file::

    type HasConfigStruct interface {
            ConfigStruct() interface{}
    }

Any plugin that implements this method should return a struct that can act as
the schema for the plugin configuration. Heka's config loader will then try to
decode the plugin's TOML config into this struct. Note that this also gives
you a way to specify default config values; you just populate your config
struct as desired before returning it from the `ConfigStruct` method.

Let's say we wanted to write a `UdpOutput` that delivered messages to a UDP
listener somewhere, defaulting to using my.example.com:44444 as the
destination. The initialization code might look as follows::

    // This is our plugin struct.
    type UdpOutput struct {
        conn net.Conn
    }

    // This is our plugin's config struct
    type UdpOutputConfig struct {
        Address string
    }

    // Provides pipeline.HasConfigStruct interface.
    func (o *UdpOutput) ConfigStruct() interface{} {
        return &UdpOutputConfig{"my.example.com:44444"}
    }

    // Initialize UDP connection
    func (o *UdpOutput) Init(config interface{}) (err error) {
        conf := config.(*UdpOutputConfig) // assert we have the right config type
        var udpAddr *net.UDPAddr
        if udpAddr, err = net.ResolveUDPAddr("udp", conf.Address); err != nil {
            return fmt.Errorf("can't resolve %s: %s", conf.Address,
                err.Error())
        }
        if o.conn, err = net.DialUDP("udp", nil, udpAddr); err != nil {
            return fmt.Errorf("error dialing %s: %s", conf.Address,
                err.Error())
        }
        return
    }

.. _inputs:

Inputs
======

Input plugins are responsible for injecting messages into the Heka pipeline.
They might be passively listening for incoming network data, actively scanning
external sources (either on the local machine or over a network), or even just
creating messages from nothing based on triggers internal to the `hekad`
process. The input plugin interface is very simple::

    type Input interface {
            Read(pipelinePack *PipelinePack, timeout *time.Duration) error
    }

As you can see, there is only a single `Read` method that accepts a pointer to
a `PipelinePack` (into which the message data should be written) and a pointer
to a `time.Duration` (which specifies how much time the read operation should
allow to pass before a timeout is considered to have occurred). The only
return value is an error (or `nil` if the read succeeds).

Note that it is very important that your input plugin honors the specified
read timeout value by returning an appropriate error if the duration elapses
before the input can get the requested data. Heka creates a fixed number of
pipeline goroutines, and if your input's `Read` method never returns, then it
will be tying up one of these goroutines, effectively removing it from the
pool.

An input plugin that reads successfully can either output raw message bytes or
a fully decoded `Message` struct object. In the former case, the message bytes
should be written into the `pipelinePack.MsgBytes` byte slice attribute. In
the latter case, the `pipelinePack.Message` object should be populated w/ the
appropriate values, and the `pipelinePack.Decoded` attribute should be set to
`true` to indicate that further decoding is not required.

In either case, for efficiency's sake, it is important to ensure that you are
actually writing the data into the memory that has already been allocated by
the `pipelinePack` struct, rather than creating new objects and repointing the
`pipelinePack` attributes to the ones you've created. Creating new objects
each time will end up causing a lot of allocation and garbage collection to
occur, which will hurt Heka performance. A lot of care has been put into the
Heka pipeline code to reuse allocated memory where possible in order to
minimize garbage collector performance impact, but a poorly written plugin can
undo these efforts and cause significant (and unnecessary) slowdowns.

If an input generates raw bytes and wishes to explicitly specify which decoder
should be used (overriding the specified default), the input can modify the
`pipelinePack.Decoder` string value. The value chosen here *must* be one of
the keys of the `pipelinePack.Decoders` map or there will be an error
condition and the message will not be processed. And, obviously, the decoder
in question must know how to work with the provided message bytes, or the
decoding will fail, again resulting in the message being lost.

.. _decoders:

Decoders
========

Decoder plugins are responsible for converting raw bytes containing message
data into actual `Message` struct objects that the Heka pipeline can process.
As with inputs, the `Decoder` interface is quite simple::

    type Decoder interface {
            Decode(pipelinePack *PipelinePack) error
    }

A decoder's `Decode` method should extract the raw message data from
`pipelinePack.MsgBytes` and attempt to deserialize this and use the contained
information to populate the Message struct pointed to by the
`pipelinePack.Message` attribute. Again, to minimize GC churn, take care to
reuse the already allocated memory rather than creating new objects and
overwriting the existing ones.

If the message bytes are decoded successfully then `Decode` should return
`nil`. If not, then an appropriate error should be returned, in which case the
error message will be logged and the message will be dropped, no further
pipeline processing will occur.

.. _filters:

Filters
=======

As with inputs and decoders, the filter plugin interface is just a single
method::

    type Filter interface {
            FilterMsg(pipelinePack *PipelinePack)
    }

The `pipelinePack` (which, by the time filters are invoked, should always
contain a valid decoded Message struct pointed to by `pipelinePack.Message`)
will be passed by the Heka pipeline engine into the filter plugin, where the
filter can perform its intended task, making any changes to either the Message
or to any other values stored on the pipelinePack to influence further
processing.

"Intended task" is pretty vague, however. What task does a filter perform,
exactly? The specific function performed by a filter plugin is not as narrowly
or clearly defined as those of inputs or decoders. Filters are where the bulk
of Heka's message processing takes place and, as such, a filter might be
performing one of any number of possible jobs:

Filtering
    As the name suggests, one possible action a filter plugin can take is to
    block a message from any further processing. This immediately scraps the
    message, preventing it from being passed to any further filters or to any
    output plugins. This is accomplished by setting `pipelinePack.Blocked` to
    `true`.

Output Selection
    The set of output plugins to which the message will be passed is indicated
    by the `pipelinePack.OutputNames` map. Any filter can change the set of
    outputs for a given message by adding or removing keys to or from this
    set.

Message Injection
    A filter might possibly watch the pipeline for certain events to happen so
    that, when triggered, a new message is generated. This can be done by
    making use of `MessageGenerator` API (global to the `pipeline` package),
    as in this example::

        msgHolder := pipeline.MessageGenerator.Retrieve()
        msgHolder.Message.Type = "yourtype"
        msgHolder.Message.Payload = "Your message payload"
        pipeline.MessageGenerator.Inject(msgHolder)

Counting / Aggregation / Roll-ups
    In some cases you might want to count the number of messages of a
    particular type that pass through a Heka pipeline. One possible way to
    handle this is to implement a filter that does the counting. The filter
    could also perform simple roll-up operations by swallowing the original
    individual messages and using message injection to generate messages
    representing the aggregate.

Event / Anomaly Detection
    A filter might be coded to watch for certain message types or message
    events such that it notices when specific behavior is (or isn't)
    happening. A simple example of this would be if an app generated a
    heartbeat message at regular intervals, a filter might be expecting these
    and would then notice if the heartbeats stopped arriving. This can be
    combined with message injection to generate notifications.

Note that this is merely a list of some of the more common uses for Heka
filter plugins. It is certainly not meant to be a comprehensive list of what
filters can do. A filter can perform any message processing that you can code.

.. _outputs:

Outputs
=======

Finally we come to the output plugins, which are responsible for receiving
Heka messages and using them to generate interactions with the outside world.
As with the other plugin types, the `Output` interface is simple, only a
single method::

    type Output interface {
            Deliver(pipelinePack *PipelinePack)
    }

The `Deliver` method's job should be obvious: extract desired message
information from the `pipelinePack` and send it on to the intended
destination. In trivial cases this is straightforward, such as this example
which simply writes the message payload out using Go's `log` module::

    type (self *LogOutput) Deliver(pipelinePack *PipelinePack) {
            log.Println(pipelinePack.Message.Payload)
    }

Most output requirements aren't trivial, however. Output plugins often require
a connection resource that must be shared among the message pipelines. A
connection sharing system could be implemented by hand using the
`PluginGlobal` and `PluginWithGlobal` mechanism described above, but this is
such a common requirement that Heka goes even further and provides something
called the `Runner` plugin to do this for you.

Registering Your Plugin
=======================

The last step you have to take after implementing your plugin is to register
it with `hekad` so it can actually be configured and used. Heka defines an
`AvailablePlugins` map for this purpose, with default entries such as these::

    var AvailablePlugins = map[string]func() interface{}{
            "UdpInput":       func() interface{} { return new(UdpInput) },
            "JsonDecoder":    func() interface{} { return new(JsonDecoder) },
            "MsgPackDecoder": func() interface{} { return new(MsgPackDecoder) },
            "StatsdUdpInput": func() interface{} { return RunnerMaker(new(StatsdInWriter)) },
            "LogOutput":      func() interface{} { return new(LogOutput) },
            "CounterOutput":  func() interface{} { return new(CounterOutput) },
            "FileOutput":     func() interface{} { return RunnerMaker(new(FileWriter)) },
    }

The `AvailablePlugins` map keys are string identifiers that can be used in
Heka's JSON config file (see :ref:`configuration`). The values are factory
functions that should create and return a new instance of the right plugin
type. In order to add `hekad` support for custom plugins, you need to add your
own entries to this map. The hekad main package contains a
`plugin_loader.go.in <https://github.com /mozilla-
services/heka/blob/dev/hekad/plugin_loader.go.in>`_ file, which you can copy
to `plugin_loader.go` and edit for this purpose.

A custom `plugin_loader.go` file might look like this::

    package main

    import (
            "github.com/mozilla-services/heka/pipeline"
            "github.com/myaccount/mypackage"
    )

    // Add custom plugins to `AvailablePlugins`, one at a time
    func init() {
            pipeline.AvailablePlugins["my_input_1"] = func() interface{} {
                    return new(mypackage.CustomInput)
            }

            pipeline.AvailablePlugins["my_input_2"] = func() interface{} {
                    return new(mypackage.CustomInput)
            }

            pipeline.AvailablePlugins["my_output"] = func() interface{} {
                    return pipeline.RunnerMaker(new(mypackage.CustomWriter))
            }
    }

Note that for simple plugins the factory function can just create an instance
and return it, but if you'd like to use the built in `Runner` plugin with a
custom `Writer` or `BatchWriter` the factory function should delegate to the
`pipeline.RunnerMaker` function, passing in an instance of the custom writer.

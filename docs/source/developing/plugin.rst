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
<https://github.com/mozilla-services/heka/blob/master/pipeline/config.go>`_
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

Input plugins are responsible for acquiring data from the outside world and
injecting this data into the Heka pipeline. An input might be passively
listening for incoming network data or actively scanning external sources
(either on the local machine or over a network). The input plugin interface
is::

    type Input interface {
            Run(ir InputRunner, h PluginHelper) (err error)
            Stop()
    }

The `Run` method is called when Heka starts and, if all is functioning as
intended, should not return until Heka is shut down. If a condition arises
such that the input can not perform its intended activity it should return
with an appropriate error, otherwise it should continue to run until a
shutdown event is triggered by Heka calling the input's `Stop` method, at
which time any clean-up should be done and a clean shutdown should be
indicated by returning a `nil` error.

Inside the `Run` method, an input has three primary responsibilities::

1. Acquire information from the outside world
2. Use acquired information to populate a `PipelinePack` object that can be
   processed by Heka.
3. Pass the populated `PipelinePack` objects on to the appropriate next stage
   in the Heka pipeline (usually to a decoder plugin so raw input data can be
   converted to a `Message` object.)

The details of the first step are clearly entirely defined by the plugin's
intended input mechanism(s). Plugins can (and should!) spin up goroutines as
needed to perform tasks such as listening on a network connection, making
requests to external data sources, scanning machine resources and operational
characteristics, reading files from a file system, etc.

For the second step, before you can populate a `PipelinePack` object you have
to actually *have* one. You can get empty packs from a channel provided to you
by the `InputRunner`. You get the channel itself by calling `ir.InChan()` and
then pull a pack from the channel whenever you need one.

Often, populating a `PipelinePack` is as simple as storing the raw data that
was retrieved from the outside world in the pack's `MsgBytes` attribute. For
efficiency's sake, it's best to write directly into the already allocated
memory rather than overwriting the attribute with a `[]byte` slice pointing to
a new array. Overwriting the array is likely to lead to a lot of garbage
collector churn.

The third step involves the input plugin deciding where next to pass the
`PipelinePack` and then doing so. Once the `MsgBytes` attribute has been set
the pack will typically be passed on to a decoder plugin, which will convert
the raw bytes into a `Message` object, also an attribute of the
`PipelinePack`. An input can gain access to the decoders that are available by
calling `PluginHelper.DecoderSet()`, which can be used to access decoders
either by the name they have been registered as in the config, or by the Heka
protocol's encoding header they have been specified as decoding.

It is up to the input to decide which decoder should be used. Once the decoder
has been determined and fetched from the `DecoderSet` the input should call
`decoder.InChan()` to fetch the input channel upon which the `PipelinePack`
can be placed.

Sometimes the input itself might wish to decode the data, rather than
delegating that job to a separate decoder. In this case the input can directly
populate the `pack.Message` and set the `pack.Decoded` value as `true`, as a
decoder would do. Decoded messages are then typically passed along to the main
Heka message router to be delivered to the appropriate filter and output
plugins. The router is available via the `PluginHelper.Router()`, and messages
are passed in by dropping the `PipelinePack` on to the router's input channel,
available via the router's `InChan` method.

One final important detail: if for any reason your input plugin should pull a
`PipelinePack` off of the input channel and *not* end up passing it on to
another step in the pipeline (i.e. to a decoder or to the router), you *must*
call `PipelinePack.Recycle()` to free the pack up to be used again. Failure to
do so will cause the `PipelinePack` pool to be depleted and will cause Heka to
freeze.

.. _decoders:

Decoders
========

Decoder plugins are responsible for converting raw bytes containing message
data into actual `Message` struct objects that the Heka pipeline can process.
As with inputs, the `Decoder` interface is quite simple::

    type Decoder interface {
            Decode(pack *PipelinePack) error
    }

A decoder's `Decode` method should extract the raw message data from
`pack.MsgBytes` and attempt to deserialize this and use the contained
information to populate the Message struct pointed to by the `pack.Message`
attribute. Again, to minimize GC churn, take care to reuse the already
allocated memory rather than creating new objects and overwriting the existing
ones.

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
services/heka/blob/master/hekad/plugin_loader.go.in>`_ file, which you can copy
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

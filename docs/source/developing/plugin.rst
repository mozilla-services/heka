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

.. _restarting_plugin:

Restarting Plugins
==================

In the event that your plugin fails to initialize properly at startup,
hekad will exit. However, once hekad is running, if a plugin should
fail (perhaps because a network connection dropped, a file became
unavailable, etc), then hekad will shutdown. This shutdown can be
avoided if your plugin supports being restarted.

To add restart support to your plugin, the `Restarting` interface
defined in the `config.go
<https://github.com/mozilla-services/heka/blob/master/pipeline/config.go>`_
file::

    type Restarting interface {
        CleanupForRestart()
    }

A plugin that implements this interface will not trigger shutdown
should it fail while hekad is running. The `CleanupForRestart` method
will be called when the plugins' main run method exits, a single time.
Then the runner will repeatedly call the plugins Init method until it
initializes successfully. It will then resume running it unless it
exits again at which point the restart process will begin anew.

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

In addition to specifying configuration options that are specific to your
plugin, it is also possible to use the config struct to specify default values
for the `ticker_interval` and `message_matcher` values that are available to
all Filter and Output plugins. If a config struct contains a uint attribute
called `TickerInterval`, that will be used as a default ticker interval value
(in seconds) if none is supplied in the TOML. Similarly, if a config struct
contains a string attribute called `MessageMatcher`, that will be used as the
default message routing rule if none is specified in the configuration file.

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
2. Use acquired information to populate `PipelinePack` objects that can be
   processed by Heka.
3. Pass the populated `PipelinePack` objects on to the appropriate next stage
   in the Heka pipeline (either to a decoder plugin so raw input data can be
   converted to a `Message` object, or by injecting them directly into the
   Heka message router if the `Message` object is already populated.)

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
decoder would do. Decoded messages are then injected into Heka's routing
system by calling `InputRunner.Inject(pack)`. The message will then be
delivered to the appropriate filter and output plugins.

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

Filter plugins are the message processing engine of the Heka system. They are
used to examine and process message contents, and trigger events based on
those contents in real time as messages are flowing through the Heka system.

The filter plugin interface is just a single method::

    type Filter interface {
            Run(r FilterRunner, h PluginHelper) (err error)
    }

Like input plugins, filters have a `Run` method which accepts a runner and a
helper, and which should not return until shutdown unless there's an error
condition. And like input plugins, filters should call `runner.InChan()` to
gain access to the plugin's input channel.

The similarities end there, however. A filter's input channel provides
pointers to `PipelinePack` objects, defined in `pipeline_runner.go
<https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_

The `Pack` contains a fully decoded `Message` object from which the
filter can extract any desired information.

Upon processing a message, a filter plugin can perform any of three tasks:

1. Pass the original message through unchanged to one or more specific
   alternative Heka filter or output plugins.
2. Generate one or more *new* messages, which can be passed to either a
   specific set of Heka plugins, or which can be handed back to the router to
   be checked against all registered plugins' `message_matcher` rules.
3. Nothing (e.g. when performing counting / aggregation / roll-ups).

To pass a message through unchanged, a filter can call `PluginHelper.Filter()`
or `PluginHelper.Output()` to access a filter or output plugin, and then call
that plugin's `Deliver()` method, passing in the `PipelinePack`.

To generate new messages, your filter must call
`PluginHelper.PipelinePack(msgLoopCount int)`. The `msgloopCount` value to be
passed in should be obtained from the `MsgLoopCount` value on the
`PipelinePack` that you're already holding, or possibly zero if the new
message is being triggered by a timed ticker instead of an incoming message.
The `PipelinePack` method will either return a pack ready for you to populate
or `nil` if the loop count is greater than the configured maximum value, as a
safeguard against inadvertently creating infinite message loops.

Once a `PipelinePack` has been obtained, a filter plugin can populate its
`Message` object. The pack can then be passed along to a specific plugin (or
plugins) as above. Alternatively, the pack can be injected into the Heka
message router queue, where it will be checked against all plugin message
matchers, by passing it to the `FilterRunner.Inject(pack *PipelinePack)`
method. Note that, again as a precaution against message looping, a plugin
will not be allowed to inject a message which would get a positive response
from that plugin's own matcher.

Sometimes a filter will take a specific action triggered by a single incoming
message. There are many cases, however, when a filter is merely collecting or
aggregating data from the incoming messages, and instead will be sending out
reports on the data that has been collected at specific intervals. Heka has
built-in support for this use case. Any filter (or output) plugin can include
a `ticker_interval` config setting (in seconds, integers only), which will
automatically be extracted by Heka when the configuration is loaded. Then from
within your plugin code you can call `FilterRunner.Ticker()` and you will get
a channel (type `<-chan time.Time`) that will send a tick at the specified
interval. Your plugin code can listen on the ticker channel and take action as
needed.

Observant readers might have noticed that, unlike the `Input` interface,
filters don't need to implement a `Stop` method. Instead, Heka will
communicate a shutdown event to filter plugins by closing the input channel
from which the filter is receiving the `PipelinePack` objects. When this
channel is closed, a filter should perform any necessary clean-up and then
return from the `Run` method with a `nil` value to indicate a clean exit.

Finally, there is one very important point that all authors of filter plugins
should keep in mind: if you are *not* passing your received `PipelinePack`
object on to another filter or output plugin for further processing, then you
*must* call `PipelinePack.Recycle()` to tell Heka that you are through with
the pack. Failure to do so will cause Heka to not free up the packs for reuse,
exhausting the supply and eventually causing the entire pipeline to freeze.

.. _outputs:

Outputs
=======

Finally we come to the output plugins, which are responsible for receiving
Heka messages and using them to generate interactions with the outside world.
The `Output` interface is nearly identical to the `Filter` interface::

    type Output interface {
            Run(or OutputRunner, h PluginHelper) (err error)
    }

In fact, there is very little difference between filter and output plugins,
other than tasks that they will be performing. Like filters, outputs should
call the `InChan` method on the provided runner to get an input channel, which
will feed `PipelinePack` objects. Like filters, outputs should listen on this 
channel until it is closed, at which time they should perform any necessary 
clean-up and thenreturn. And, like filters, any output plugin with a 
`ticker_interval` value in the configuration will use that value to create a 
ticker channel that can be accessed using the runner's `Ticker` method. And, 
finally, outputs should also be sure to call `PipelinePack.Recycle()` when 
they finish w/ a pack so that Heka knows the pack is freed up for reuse.

.. _register_custom_plugins

Registering Your Plugin
=======================

The last step you have to take after implementing your plugin is to register
it with `hekad` so it can actually be configured and used. You do this by
calling the `pipeline` package's `RegisterPlugin` function::

    func RegisterPlugin(name string, factory func() interface{})

The `name` value should be a unique identifier for your plugin, and it should
end in one of "Input", "Decoder", "Filter", or "Output", depending on the
plugin type.

The `factory` value should be a function that returns an instance of your
plugin, usually a pointer to a struct, where the pointer type implements the
`Plugin` interface and the interface appropriate to its type (i.e. `Input`,
`Decoder`, `Filter`, or `Output`).

This sounds more complicated than it is. Here are some examples from Heka
itself::

    RegisterPlugin("UdpInput", func() interface{} {return new(UdpInput)})
    RegisterPlugin("TcpInput", func() interface{} {return new(TcpInput)})
    RegisterPlugin("JsonDecoder", func() interface{} {return new(JsonDecoder)})
    RegisterPlugin("ProtobufDecoder", func() interface{} {return new(ProtobufDecoder)})
    RegisterPlugin("CounterFilter", func() interface{} {return new(CounterFilter)})
    RegisterPlugin("StatFilter", func() interface{} {return new(StatFilter)})
    RegisterPlugin("LogOutput", func() interface{} {return new(LogOutput)})
    RegisterPlugin("FileOutput", func() interface{} {return new(FileOutput)})

It is recommended that `RegisterPlugin` calls be put in your Go package's
`init() function <http://golang.org/doc/effective_go.html#init>`_ so that you
can simply import your package when building `hekad` and the package's plugins
will be registered and available for use in your Heka config file. This is
made a bit easier if you use `heka build`_, see
:ref:`build_include_externals`.

.. _heka build: https://github.com/mozilla-services/heka-build

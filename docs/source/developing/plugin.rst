.. _plugins:

==============
Extending Heka
==============

The core of the Heka engine is written in the `Go <http://golang.org>`_
programming language. Heka supports five different types of plugins (inputs,
decoders, filters, encoders, and outputs), which are also written in Go. This
document will try to provide enough information for developers to extend Heka
by implementing their own custom plugins. It assumes a small amount of
familiarity with Go, although any reasonably experienced programmer will
probably be able to follow along with no trouble.

*NOTE*: Heka also supports the use of security sandboxed `Lua
<http://www.lua.org>`_ code for implementing the core logic of decoder,
filter, and encoder plugins. This document only covers the development of Go
plugins. You can learn more about sandboxed plugins in the :ref:`sandbox`
section.

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
processing of Heka message data, encoders serialize Heka messages into
arbitrary byte streams, and outputs send data from Heka back to the outside
world. Each specific plugin has some custom behaviour, but it also shares
behaviour w/ every other plugin of that type. A UDPInput and a TCPInput listen
on the network differently, and a LogstreamerInput (reading files off the file
system) doesn't listen on the network at all, but all of these inputs need to
interact w/ the Heka system to access data structures, gain access to decoders
to which we pass our incoming data, respond to shutdown and other system
events, etc.

To support this all Heka plugins except encoders actually consist of two
parts: the plugin itself, and an accompanying "plugin runner". Inputs have an
InputRunner, decoders have a DecoderRunner, filters have a FilterRunner, and
Outputs have an OutputRunner. The plugin itself contains the plugin-specific
behaviour, and is provided by the plugin developer. The plugin runner contains
the shared (by type) behaviour, and is provided by Heka. When Heka starts a
plugin, it a) creates and configures a plugin instance of the appropriate
type, b) creates a plugin runner instance of the appropriate type (passing in
the plugin), and c) calls the Start method of the plugin runner. Most plugin
runners (all except decoders) then call the plugin's Run method, passing
themselves and an additional PluginHelper object in as arguments so the plugin
code can use their exposed APIs to interact w/ the Heka system.

For inputs, filters, and outputs, there's a 1:1 correspondence between
sections specified in the config file and running plugin instances. This is
not the case for decoders and encoders, however. Decoder and encoder sections
register possible configurations, but actual decoder and encoder instances
aren't created until they are used by input or output plugins.

.. _plugin_config:

Plugin Configuration
====================

Heka uses a slightly modified version of `TOML
<https://github.com/mojombo/toml>`_ as its configuration file format (see:
:ref:`configuration`), and provides a simple mechanism through which plugins
can integrate with the configuration loading system to initialize themselves
from settings in hekad's config file.

The minimal shared interface that a Heka plugin must implement in order to use
the config system is (unsurprisingly) `Plugin`, defined in `pipeline_runner.go
<https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_::

    type Plugin interface {
        Init(config interface{}) error
    }

During Heka initialization an instance of every plugin listed in the
configuration file will be created. The TOML configuration for each plugin
will be parsed and the resulting configuration object will be passed in to the
above specified `Init` method. The argument is of type `interface{}`. By
default the underlying type will be `*pipeline.PluginConfig`, a map object
that provides config data as key/value pairs. There is also a way for plugins
to specify a custom struct to be used instead of the generic `PluginConfig`
type (see :ref:`custom_plugin_config`). In either case, the config object will
be already loaded with values read in from the TOML file, which your plugin
code can then use to initialize itself. The input, filter, and output plugins
will then be started so they can begin processing messages. The decoder and
encoder instances will be thrown away, with new ones created as needed when
requested by input (for decoder) or output (for encoder) plugins.

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

In the event that your plugin fails to initialize properly at startup, hekad
will exit. However, once hekad is running, if the plugin should fail (perhaps
because a network connection dropped, a file became unavailable, etc) then the
plugin will exit. If the plugin supports being restarted then Heka will
attempt to reset, reinitialize, and restart the plugin. If this fails, Heka
will try again up until the specified max_retries value. If the failure
continues beyond the maximum number of retries, or if the plugin didn't
support restarting in the first place, then Heka will either shut down or, if
the plugin is a filter or an output with the `can_exit` setting set to true,
the plugin will be removed from operation and Heka will continue to run.

If the reinitialization and restarting is successful, then the retry count
will be reset to zero and everything will continue to function as normal.

To add restart support to your plugin, you must implement the `Restarting`
interface defined in the `config.go <https://github.com/mozilla-
services/heka/blob/master/pipeline/config.go>`_ file::

    type Restarting interface {
        CleanupForRestart()
    }

The `CleanupForRestart` method will be called when the plugin's main run
method exits, a single time. This allows you a place to perform any additional
cleanup that might be necessary before attempting to reinitialize the plugin.
After this, the runner will repeatedly call the plugin's Init method until it
initializes successfully. It will then resume running it unless it exits again
at which point the restart process will begin anew.

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

Let's look at the code for Heka's `UdpOutput`, which delivers messages to a
UDP listener somewhere. The initialization code looks as follows::

    // This is our plugin struct.
    type UdpOutput struct {
        *UdpOutputConfig
        conn net.Conn
    }

    // This is our plugin's config struct
    type UdpOutputConfig struct {
        // Network type ("udp", "udp4", "udp6", or "unixgram"). Needs to match the
        // input type.
        Net string
        // String representation of the address of the network connection to which
        // we will be sending out packets (e.g. "192.168.64.48:3336").
        Address string
        // Optional address to use as the local address for the connection.
        LocalAddress string `toml:"local_address"`
    }

    // Provides pipeline.HasConfigStruct interface.
    func (o *UdpOutput) ConfigStruct() interface{} {
        return &UdpOutputConfig{
            Net: "udp",
        }
    }

    // Initialize UDP connection
    func (o *UdpOutput) Init(config interface{}) (err error) {
        o.UdpOutputConfig = config.(*UdpOutputConfig) // assert we have the right config type

        if o.Net == "unixgram" {
            if runtime.GOOS == "windows" {
                return errors.New("Can't use Unix datagram sockets on Windows.")
            }
            var unixAddr, lAddr *net.UnixAddr
            unixAddr, err = net.ResolveUnixAddr(o.Net, o.Address)
            if err != nil {
                return fmt.Errorf("Error resolving unixgram address '%s': %s", o.Address,
                    err.Error())
            }
            if o.LocalAddress != "" {
                lAddr, err = net.ResolveUnixAddr(o.Net, o.LocalAddress)
                if err != nil {
                    return fmt.Errorf("Error resolving local unixgram address '%s': %s",
                        o.LocalAddress, err.Error())
                }
            }
            if o.conn, err = net.DialUnix(o.Net, lAddr, unixAddr); err != nil {
                return fmt.Errorf("Can't connect to '%s': %s", o.Address,
                    err.Error())
            }
        } else {
            var udpAddr, lAddr *net.UDPAddr
            if udpAddr, err = net.ResolveUDPAddr(o.Net, o.Address); err != nil {
                return fmt.Errorf("Error resolving UDP address '%s': %s", o.Address,
                    err.Error())
            }
            if o.LocalAddress != "" {
                lAddr, err = net.ResolveUDPAddr(o.Net, o.LocalAddress)
                if err != nil {
                    return fmt.Errorf("Error resolving local UDP address '%s': %s",
                        o.Address, err.Error())
                }
            }
            if o.conn, err = net.DialUDP(o.Net, lAddr, udpAddr); err != nil {
                return fmt.Errorf("Can't connect to '%s': %s", o.Address,
                    err.Error())
            }
        }
        return
    }

In addition to specifying configuration options that are specific to your
plugin, it is also possible to use the config struct to specify default values
for any common configuration options that are processed by Heka, such as the
`synchronous_decode` option available to Input plugins, or the
`ticker_interval` and `message_matcher` values that are available to all
Filter and Output plugins. If a config struct contains a uint attribute called
`TickerInterval`, that will be used as a default ticker interval value (in
seconds) if none is supplied in the TOML. Similarly, if a config struct
contains a string attribute called `MessageMatcher`, that will be used as the
default message routing rule if none is specified in the configuration file.

There is an optional configuration interface called WantsName.  It provides a
a plugin access to its configured name before the runner has started. The
SandboxFilter plugin uses the name to locate/load any preserved state before
being run::

    type WantsName interface {
        SetName(name string)
    }

There is also a similar WantsPipelineConfig interface that can be used if a
plugin needs access to the active PipelineConfig or GlobalConfigStruct values
in the ConfigStruct or Init methods. (If these values are needed in the Run
method they can be retrieved from the PluginRunner.)::

    type WantsPipelineConfig interface {
        SetPipelineConfig(pConfig *pipeline.PipelineConfig)
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

Inside the `Run` method, an input has three primary responsibilities:

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

In some cases, populating a `PipelinePack` is as simple as storing the raw
data that was retrieved from the outside world in the pack's `MsgBytes`
attribute. For efficiency's sake, it's best to write directly into the already
allocated memory rather than overwriting the attribute with a `[]byte` slice
pointing to a new array. Overwriting the array is likely to lead to a lot of
garbage collector churn. Right now the ProtobufDecoder is the only decoder
that expects to find its input data in the MsgBytes attribute of the pack.

In all other cases, the decoder will expect to find its input data stored as
the payload of the pack's Message struct. This should also be the case when
there is no decoder in use, since it allows the rest of the Heka pipeline to
access the raw textual data that was loaded by the input plugin.

The third step is often very simple. Usually, input plugins merely need to
pass the now populated PipelinePack to the InputRunner's `Deliver` method. If
the input's configuration specified a decoder, then the pack will be passed in
to an instance of the specified decoder, either synchronously or via a Go
channel, depending on the value of the `synchronous_decode` config setting.
After decoding, the decoded pack (and its contained message) will be passed on
to Heka's message router. If no decoder was specified, then calling `Deliver`
causes the pack to be passed directly to the router.

One limitation of relying on the InputRunner's Deliver method to handle the
decoding and delivery is that each input plugin will get only a single decoder
instance. This is usually sufficient, but occasionally an input will spin up
multiple input goroutines, and it may require multiple decoders to adequately
handle the input streams. In this case, an input can implement the
`pipeline.DoesOwnDecoding` interface, which consists of a single method::

    SetCommonInputConfig(commonInputConfig pipeline.CommonInputConfig)

If an input has this method, then Heka will not create a decoder for the
input, but instead will call SetCommonInputConfig just before calling the Run
method, passing in a CommonInputConfig struct populated with values extracted
from the input's TOML configuration. CommonInputConfig structs are defined as
follows::

    type CommonInputConfig struct {
        Ticker             uint `toml:"ticker_interval"`
        Decoder            string
        SyncDecode         *bool `toml:"synchronous_decode"`
        SendDecodeFailures *bool `toml:"send_decode_failures"`
        Retries            RetryOptions
    }

This provides all of the information an input needs to handle decoding and
delivery according to the provided configuration. The `Decoder`, `SyncDecode`,
and `SendDecodeFailures` attributes tell which decoder to use, whether the
decoding should happen synchronously, and whether messages that fail decoding
should be tagged and delivered to the router, respectively.

If a decoder is specified and SyncDecode is nil or false, an input can use the
PluginHelper's DecoderRunner method to get a decoder running in its own
goroutine. Messages are passed in to the decoder via a channel provided by the
DecoderRunner's InChan() method. The input code is responsible for calling
DecoderRunner.SetSendFailure(), passing in the value of the SendDecodeFailures
setting. If a DecoderRunner is no longer needed,
`PluginHelper.StopDecoderRunner` should be called to shut it down or else it
will continue to consume system resources throughout the life of the Heka
process.

If a decoder is specified and SyncDecoder is true, an input can call
PluginHelper.PipelineConfig().Decoder() to get an unwrapped decoder object.
The input should call Decode directly, and should also honor the specified
SendDecodeFailures behaviour. If decode failures are sent, the input should
pass the pack's Message struct in to the `pipeline.AddDecodeFailureField` to
tag it as a decoding failure before delivering it to the router using the
InputRunner's Inject() method.

If no decoder is specified, then the input should simply pass the populated
pack directly to the router using InputRunner.Inject().

One final important detail: if for any reason your input plugin should pull a
`PipelinePack` off of the input channel and *not* end up passing it on to
another step in the pipeline (i.e. by calling `Deliver` or passing it to a
decoder or to the router), you *must* call `PipelinePack.Recycle()` to free
the pack up to be used again. Failure to do so will cause the `PipelinePack`
pool to be depleted and will cause Heka to freeze.

.. _decoders:

Decoders
========

Decoder plugins are responsible for converting raw bytes containing message
data into actual `Message` struct objects that the Heka pipeline can process.
As with inputs, the `Decoder` interface is quite simple::

    type Decoder interface {
        Decode(pack *PipelinePack) (packs []*PipelinePack, err error)
    }

There are two optional Decoder interfaces.  The first provides the Decoder
access to its DecoderRunner object when it is started::

    type WantsDecoderRunner interface {
        SetDecoderRunner(dr DecoderRunner)
    }

The second provides a notification to the Decoder when the DecoderRunner is
exiting::

    type WantsDecoderRunnerShutdown interface {
        Shutdown()
    }

A decoder's `Decode` method should extract the raw message data from
`pack.MsgBytes` and attempt to deserialize this and use the contained
information to populate the Message struct pointed to by the `pack.Message`
attribute. Again, to minimize GC churn, take care to reuse the already
allocated memory rather than creating new objects and overwriting the existing
ones.

If the message bytes are decoded successfully then `Decode` should return a
slice of PipelinePack pointers and a nil error value. The first item in the
returned slice (i.e. `packs[0]`) should be the pack that was passed in to the
method. If the decoding process produces more than one output pack, additonal
packs can be appended to the slice.

If decoding fails for any reason, then `Decode` should return a nil value for
the PipelinePack slice, causing the message to be dropped with no further
processing. Returning an appropriate error value will cause Heka to log an
error message about the decoding failure.

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

.. _encoders:

Encoders
========

Encoder plugins are the inverse of decoders. They convert `Message` structs
into raw bytes that can be delivered to the outside world. Some encoders will
serialize an entire `Message` struct, such as the :ref:`config_protobufencoder`
which uses Heka's native protocol buffers format. Other encoders extract data
from the message and insert it into a different format such as plain text or
JSON.

The `Encoder` interface consists of one method::

    Encode(pack *PipelinePack) (output []byte, err error)

This method accepts a PiplelinePack containing a populated message object and
returns a byte slice containing the data that should be sent out, or an error
if serialization fails for some reason. If the encoder wishes to swallow an
input message without generating any output (such as for batching, or because
the message contains no new data) then nil should be returned for both the
output and the error.

Unlike the other plugin types, encoders don't have a PluginRunner, nor do they
run in their own goroutines. Outputs invoke encoders directly, by calling the
Encode method exposed on the OutputRunner. This has the same signature as the
Encoder interface's Encode method, to which it will will delegate. If
`use_framing` is set to true in the output's configuration, however, the
OutputRunner will prepend Heka's :ref:`stream_framing` to the generated binary
data.

Outputs can also directly access their encoder instance by calling
OutputRunner.Encoder(). Encoders themselves don't handle the stream framing,
however, so it is recommended that outputs use the OutputRunner method
instead.

Even though encoders don't run in their own goroutines, it is possible that
they might need to perform some clean up at shutdown time. If this is so, the
encoder can implement the `NeedsStopping` interface::

    Stop()

And the `Stop` method will be called during the shutdown sequence.

.. _outputs:

Outputs
=======

Finally we come to the output plugins, which are responsible for receiving
Heka messages and using them to generate interactions with the outside world.
The `Output` interface is nearly identical to the `Filter` interface::

    type Output interface {
        Run(or OutputRunner, h PluginHelper) (err error)
    }

In fact, there are many ways in which filter and output plugins are similar.
Like filters, outputs should call the `InChan` method on the provided runner
to get an input channel, which will feed `PipelinePack` objects. Like filters,
outputs should listen on this channel until it is closed, at which time they
should perform any necessary clean-up and then return. And, like filters, any
output plugin with a `ticker_interval` value in the configuration will use
that value to create a ticker channel that can be accessed using the runner's
`Ticker` method. And, finally, outputs should also be sure to call
`PipelinePack.Recycle()` when they finish w/ a pack so that Heka knows the
pack is freed up for reuse.

The primary way that outputs differ from filters, of course, is that outputs
need to serialize (or extract data from) the messages they receive and then
send that data to an external destination. The serialization (or data
extraction) should typically be performed by the output's specified encoder
plugin. The OutputRunner exposes the following methods to assist with this::

    Encode(pack *PipelinePack) (output []byte, err error)
    UsesFraming() bool
    Encoder() (encoder Encoder)

The Encode method will use the specified encoder to convert the pack's message
to binary data, then if `use_framing` was set to true in the output's
configuration it will prepend Heka's :ref:`stream_framing`. The UsesFraming
method will tell you whether or not `use_framing` was set to true. Finally,
the Encoder method will return the actual encoder that was registered. This is
useful to check to make sure that an encoder was actually registered, but
generally you will want to use OutputRunner.Encode and not Encoder.Encode,
since the latter will not honor the output's `use_framing` specification.

.. _register_custom_plugins:

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
    RegisterPlugin("ProtobufDecoder", func() interface{} {return new(ProtobufDecoder)})
    RegisterPlugin("CounterFilter", func() interface{} {return new(CounterFilter)})
    RegisterPlugin("StatFilter", func() interface{} {return new(StatFilter)})
    RegisterPlugin("LogOutput", func() interface{} {return new(LogOutput)})
    RegisterPlugin("FileOutput", func() interface{} {return new(FileOutput)})

It is recommended that `RegisterPlugin` calls be put in your Go package's
`init() function <http://golang.org/doc/effective_go.html#init>`_ so that you
can simply import your package when building `hekad` and the package's plugins
will be registered and available for use in your Heka config file. This is
made a bit easier if you use `plugin_loader.cmake`, see
:ref:`build_include_externals`.

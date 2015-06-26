.. _plugins:

==============
Extending Heka
==============

The core of the Heka engine is written in the `Go <http://golang.org>`_
programming language. Heka supports six different types of plugins (inputs,
splitters, decoders, filters, encoders, and outputs), which are also written
in Go. This document will try to provide enough information for developers to
extend Heka by implementing their own custom plugins. It assumes a small
amount of familiarity with Go, although any reasonably experienced programmer
will probably be able to follow along with no trouble.

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

Each Heka plugin type performs a specific task. Inputs receive input from the
outside world and inject the data into the Heka pipeline. Splitters slice the
input stream into individual records. Decoders turn binary data into Message
objects that Heka can process. Filters perform arbitrary processing of Heka
message data. Encoders serialize Heka messages into arbitrary byte streams.
Outputs send data from Heka back to the outside world. Each specific plugin
has some custom behaviour, but it also shares behaviour w/ every other plugin
of that type. A UDPInput and a TCPInput listen on the network differently, and
a LogstreamerInput (reading files off the file system) doesn't listen on the
network at all, but all of these inputs need to interact w/ the Heka system to
access data structures, gain access to decoders to which we pass our incoming
data, respond to shutdown and other system events, etc.

To support this all Heka plugins except encoders actually consist of two
parts: the plugin itself, and an accompanying "plugin runner". Inputs have an
InputRunner, splitters have a SplitterRunner, decoders have a DecoderRunner,
filters have a FilterRunner, and Outputs have an OutputRunner. The plugin
itself contains the plugin-specific behaviour, and is provided by the plugin
developer. The plugin runner contains the shared (by type) behaviour, and is
provided by Heka. When Heka starts a plugin, it first creates and configures a
plugin instance of the appropriate type, then it creates a plugin runner
instance of the appropriate type, passing in the plugin.

For inputs, filters, and outputs, there's a 1:1 correspondence between
sections specified in the config file and running plugin instances. This is
not the case for splitters, decoders and encoders, however. Configuration
sections for splitter, decoder and encoder plugins register *possible*
configurations, but actual running instances of these types aren't created
until they are used by input or output plugins.

.. _plugin_config:

Plugin Configuration
====================

Heka uses a slightly modified version of `TOML
<https://github.com/mojombo/toml>`_ as its configuration file format (see:
:ref:`configuration`), and provides a simple mechanism through which plugins
can integrate with the configuration loading system to initialize themselves
from settings in hekad's config file.

The minimal shared interface that a Heka plugin must implement in order to use
the config system is (unsurprisingly) ``Plugin``, defined in
`pipeline_runner.go <https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_::

    type Plugin interface {
        Init(config interface{}) error
    }

During Heka initialization an instance of every plugin listed in the
configuration file will be created. The TOML configuration for each plugin
will be parsed and the resulting configuration object will be passed in to the
above specified ``Init`` method. The argument is of type ``interface{}``. By
default the underlying type will be ``*pipeline.PluginConfig``, a map object
that provides config data as key/value pairs. There is also a way for plugins
to specify a custom struct to be used instead of the generic `PluginConfig`
type (see :ref:`custom_plugin_config`). In either case, the config object will
be already loaded with values read in from the TOML file, which your plugin
code can then use to initialize itself. The input, filter, and output plugins
will then be started so they can begin processing messages. The splitter,
decoder, and encoder instances will be thrown away, with new ones created as
needed when requested by input (for splitter and decoder) or output (for
encoder) plugins.

As an example, imagine we're writing a filter that will deliver messages to a
specific output plugin, but only if they come from a list of approved hosts.
Both 'hosts' and 'output' would be required in the plugin's config section.
Here's one version of what the plugin definition and ``Init`` method might
look like::

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

If your plugin supports being restarted and either fails to initialize properly
at startup, or fails during Run with an error (perhaps because a network
connection dropped, a file became unavailable, etc) then Heka will attempt to
reinitialize and restart it up until the specified max_retries value.

If the failure continues beyond the maximum number of retries, or if the plugin
didn't support restarting in the first place, then Heka will either shut down
or, if the plugin is an input, filter or an output with the ``can_exit``
setting set to true, the plugin will be removed from operation and Heka will
continue to run.

To add restart support to your plugin, you must implement the ``Restarting``
interface defined in the `config.go <https://github.com/mozilla-
services/heka/blob/master/pipeline/config.go>`_ file::

    type Restarting interface {
        CleanupForRestart()
    }

The ``CleanupForRestart`` method will be called when the plugin's main run
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
the ``HasConfigStruct`` interface defined in the `config.go
<https://github.com/mozilla-services/heka/blob/master/pipeline/config.go>`_
file::

    type HasConfigStruct interface {
        ConfigStruct() interface{}
    }

Any plugin that implements this method should return a struct that can act as
the schema for the plugin configuration. Heka's config loader will then try to
decode the plugin's TOML config into this struct. Note that this also gives
you a way to specify default config values; you just populate your config
struct as desired before returning it from the ``ConfigStruct`` method.

Let's look at the code for Heka's UdpOutput, which delivers messages to a
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
``synchronous_decode`` option available to Input plugins, or the
``ticker_interval`` and ``message_matcher`` values that are available to all
filter and output plugins. If a config struct contains a uint attribute called
``TickerInterval``, that will be used as a default ticker interval value (in
seconds) if none is supplied in the TOML. Similarly, if a config struct
contains a string attribute called `MessageMatcher`, that will be used as the
default message routing rule if none is specified in the configuration file.

There is an optional configuration interface called `WantsName`.  It provides
a plugin access to its configured name before the runner has started. The
SandboxFilter plugin uses the name to locate/load any preserved state before
being run::

    type WantsName interface {
        SetName(name string)
    }

There is also a similar `WantsPipelineConfig` interface that can be used if a
plugin needs access to the active PipelineConfig or GlobalConfigStruct values
in the ConfigStruct or Init methods::

    type WantsPipelineConfig interface {
        SetPipelineConfig(pConfig *pipeline.PipelineConfig)
    }

Note that, in the case of inputs, filters, and outputs, these interfaces only
need to be implemented if you need this information *before* the plugin is
started. Once started, the plugin runner and a plugin helper will be passed in
to the Run method, which make the plugin name and PipelineConfig struct
available in other ways.

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

The ``Run`` method is called when Heka starts and, if all is functioning as
intended, should not return until Heka is shut down. If a condition arises
such that the input can not perform its intended activity it should return
with an appropriate error, otherwise it should continue to run until a
shutdown event is triggered by Heka calling the input's ``Stop`` method, at
which time any clean-up should be done and a clean shutdown should be
indicated by returning a nil error.

Inside the Run method, an input typically has three primary responsibilities:

1. Access some data or data stream from the outside world.
2. Provide acquired data or stream to a SplitterRunner for record extraction
   and further delivery.
3. (optional) Provide a "pack decorator" function to the SplitterRunner to
   populate the message object with any input-specific information.

The details of the first step are clearly entirely defined by the plugin's
intended input mechanism(s). Plugins can (and should!) spin up goroutines as
needed to perform tasks such as listening on a network connection, making
requests to external data sources, scanning machine resources and operational
characteristics, reading files from a file system, etc.

For the second step, you need to get a SplitterRunner to which you can feed
your incoming data. This is available through the InputRunner's
``NewSplitterRunner`` method. NewSplitterRunner takes a single string argument
called `token`. This token is used to differentiate multiple SplitterRunner
instances from each other. If you have a simple input plugin that only needs a
single SplitterRunner, you can just pass an empty string (i.e. ``sr :=
ir.NewSplitterRunner("")``). In more complicated scenarios you might want
multiple SplitterRunners, say one per goroutine, in which case you should pass
a unique identifier string in to each NewSplitterRunner call.

Splitting records efficiently is a surprisingly complicated process so the
SplitterRunner interface has a number of methods::

    type SplitterRunner interface {
        PluginRunner
        SetInputRunner(ir InputRunner)
        Splitter() Splitter
        SplitBytes(data []byte, del Deliverer) error
        SplitStream(r io.Reader, del Deliverer) error
        GetRemainingData() (record []byte)
        GetRecordFromStream(r io.Reader) (int, []byte, error)
        DeliverRecord(record []byte, del Deliverer)
        KeepTruncated() bool
        UseMsgBytes() bool
        SetPackDecorator(decorator func(*PipelinePack))
    }

Don't let this scare you, however. SplitterRunner's expose some internal
workings to be able to support advanced uses, but in most cases you only need
to deal with a few of the exposed methods. Specifically, you care about either
``SplitStream`` or ``SplitBytes``, and possibly about ``SetPackDecorator`` and
``UseMsgBytes``.

First we'll examine the "Split" methods. As mentioned above, you'll typically
only want to use one or the other. Deciding which you want is straightforward.
If your mechanism for getting data from the outside world is a stream object
(an `io.Reader`, in Go terms), then you'll want SplitStream. If not and you
just end up with a byte slice of binary data, then you'll want SplitBytes.

Note that both SplitStream and SplitBytes ask for a ``Deliverer`` object as
their second argument. Again, in simple cases you don't need to worry about
this. If you're only using a single SplitterRunner, you can just pass in nil
and Heka will take care of delivering the message to a decoder and/or the
message router appropriately. If you're using multiple goroutines (and
therefore multiple SplitterRunners), however, you'll typically want multiple
Deliverers, too. This is especially important if you want each separate
goroutine to have its own Decoder, so decoding can happen in parallel,
delegated to multiple cores on a single machine.

Like SplitterRunners, Deliverers are obtained from the InputRunner, using the
``NewDeliverer`` method. And, like SplitterRunners, NewDeliverer takes a
single string identifier argument, which should be unique for each requested
deliverer. Usually a single SplitterRunner will be using a single Deliverer,
and the same token identifier will be used for each. You can see an example of
this in the TcpInput's `handleConnection` code snippet a bit further down this
page.

If you're using SplitBytes, then you'll want to call it each time you have a
new payload of data to process. It will return the number of bytes
successfully consumed from the provided slice, and any relevant errors
occurred while processing. It is up to the calling code to decide what to do
in error cases, or when all of the data isn't consumed.

If you're using SplitStream, then the SplitStream call will block for as long
as it is consuming data. When data processing pauses or stops, SplitStream
will exit and return control back to the input, returning either nil or any
relevant errors. Typically if nil is returned, you'll want to call SplitStream
again to continue processing the stream. Code such as the following is a
common idiom::

    var err error
    for err == nil {
        err = sr.SplitStream(r, nil)
    }

Any errors encountered while processing the stream, including io.EOF, will be
returned from the SplitStream call. It is up to the input code to decide how
to proceed.

Finally, we're ready for the third step, providing a "pack decorator" function
to the SplitterRunner. Sometimes an input plugin would like to populate a Heka
message with information specific to the input mechanism. The TcpInput, for
instance, often wants to store the remote address of the TCP connection as a
message's Hostname field. Any provided pack decorator function will be called
immediately before the PipelinePack is passed on for delivery, allowing the
input to mutate the pack's Message struct as desired. The TcpInput code that
uses this feature looks like so::

    func (t *TcpInput) handleConnection(conn net.Conn) {
        raddr := conn.RemoteAddr().String()
        host, _, err := net.SplitHostPort(raddr)
        if err != nil {
            host = raddr
        }

        deliverer := t.ir.NewDeliverer(host)
        sr := t.ir.NewSplitterRunner(host)

        defer func() {
            conn.Close()
            t.wg.Done()
            deliverer.Done()
        }()

        if !sr.UseMsgBytes() {
            packDec := func(pack *PipelinePack) {
                pack.Message.SetHostname(raddr)
            }
            sr.SetPackDecorator(packDec)
        }

The ``if !sr.UseMsgBytes()`` check before the SetPackDecorator call deserves
some explanation. Generally Heka receives input data in one of two flavors.
The first is standalone data, usually text, such as log files loaded from the
file system using a LogstreamerInput. This data is stored within a Message
struct, usually as the payload. Most decoder plugins, then, will expect to find
the raw input data in the Message payload, and will parse this data and mutate the
Message struct with extracted data.

The second flavor of input data is a binary blob, usually protocol buffers
encoded, representing an entire Heka message. Clearly it doesn't make much
sense to store data representing a serialized Message struct *within* a
Message struct, since it would overwrite itself upon deserialization. For this
reason, PipelinePacks have a MsgBytes attribute that is used as a buffer for
storing binary data that will be converted to a message. Certain decoder
plugins, most notably the ProtobufDecoder, will expect to find input data in
the pack.MsgBytes buffer, and will use this to create a new Message struct
from scratch.

Splitters can specify via a config setting whether the data records they parse
should be placed in the message payload of an existing Message struct or in
the MsgBytes attribute of the enclosing PipelinePack, depending on what the
accompanying decoder plugin expects. The UseMsgBytes method on the
SplitterRunner will return true if the contained splitter plugin is putting
the data in the MsgBytes buffer, or false if it is putting the data in the
Message's Payload field.

Now we can understand why the TcpInput is checking this before setting the
pack decorator. When UseMsgBytes returns true, then the Message struct on that
pack is going to be overwritten when decoding happens. There's not much value
in setting the Hostname field when it's going to be clobbered shortly
afterward.

Okay, that covers most of what you need to know about developing your own Heka
input plugins. There's one important final possibility to consider, however.
In some cases, an input might fail to retrieve any data at all, so it has
nothing to hand to the Splitter. Even so, it might *still* want to deliver a
message containing information about the data retrieval failure itself. The
HttpInput does this when an HTTP request fails completely due to network or
other errors, for instance.

When this happens the input must obtain a fresh PipelinePack, manually
populate the contained Message struct, and manually hand it over for delivery.
Here's the snippet in the HttpInput code that does this::

    resp, err := httpClient.Do(req)
    responseTime := time.Since(responseTimeStart)
    if err != nil {
        pack := <-hi.ir.InChan()
        pack.Message.SetUuid(uuid.NewRandom())
        pack.Message.SetTimestamp(time.Now().UnixNano())
        pack.Message.SetType("heka.httpinput.error")
        pack.Message.SetPayload(err.Error())
        pack.Message.SetSeverity(hi.conf.ErrorSeverity)
        pack.Message.SetLogger(url)
        hi.ir.Deliver(pack)
        return
    }

As you can see, the pattern is simple. The PipelinePack supply is exposed via
a channel provided by the InputRunner's ``InChan`` method, so we pull from
this channel to get a fresh pack. Then we populate the Message struct with any
relevant data we want to include, and we finish up by passing the pack in to
the InputRunner's ``Deliver`` method for delivery. If we were using separate
Deliverers, then we would call the Deliver method on the relevant Deliverer
instance instead of on the InputRunner.

One important detail about this pattern, however: if for any reason your
plugin should pull a PipelinePack off of the input channel and *not* end up
passing it on to one of the Deliver methods, you *must* call
``pack.Recycle()`` to free the pack up to be used again. Failure to do so will
eventually deplete the pool of PipelinePacks and will cause Heka to freeze.

.. _splitters:

Splitters
=========

In contrast to the relatively complicated SplitterRunner interface that is
discussed in the :ref:`inputs` section above, the actual Splitter plugins
themselves are very simple. The basic Splitter interface consists of a single
method::

    // Splitter plugin interface type.
    type Splitter interface {
        FindRecord(buf []byte) (bytesRead int, record []byte)
    }

The job of the ``FindRecord`` method is straightforward. It should scan
through the provided byte slice, from the beginning, looking for any
delimiters or appropriate indicators of a record boundary. It returns two
values, the number of bytes consumed from the input buffer, and a slice that
represents any record that was found.  The ``bytesRead`` value should always
be returned, whether a record slice is returned or not. If the entire buffer
was scanned but no record was found, for instance, then bytesRead should be
``len(buf)``.

Note that when a record is discovered, the returned slice can (and should, if
possible) be a subsection of the input buffer. It's recommended that
FindRecord not do any unnecessary copying of the input data.

In many cases this is all that is required of a splitter plugin. In some
situations, however, records may include some headers and/or framing of some
sort, and additional processing of those headers might be called for. For
instance, Heka's native :ref:`stream_framing` can embed HMAC authenticated
message signing information in the message header, and the splitter needs to
be able to decide whether or not the authentication is valid. For this reason,
splitter plugins can implement an additional ``UnframingSplitter`` interface::

    // UnframingSplitter is an interface optionally implemented by splitter
    // plugins to remove and process any record framing that may have been used by
    // the splitter.
    type UnframingSplitter interface {
        UnframeRecord(framed []byte, pack *PipelinePack) []byte
    }

The FindRecord method of an UnframingSplitter should return the full record,
frame and all. Heka will then pass each framed record into the
``UnframeRecord`` method, along with the PipelinePack into which the record
will be written. UnframeRecord should then extract the record framing, process
it as needed, and return a byte slice containing the unframed record that is
remaining. As with FindRecord, copying the data isn't necessary, the unframed
record can safely refer to a subslice of the original framed record.

If the splitter examines the headers and decides that a given record is for
some reason not valid, such as for the use of an incorrect authentication key,
then it should return nil instead of the contained record. Additionally,
signing information can be written to the PipelinePack's ``Signer`` attribute,
and this will be honored by the ``message_signer`` config setting available to
:ref:`filter <config_common_filter_parameters>` and :ref:`output
<config_common_output_parameters>` plugins.

Note that if UnframeRecord returns nil it does *not* need to call
``pack.Recycle()``. Heka will recognize that the pack isn't going to be used and
will recycle it itself.

.. _decoders:

Decoders
========

Decoder plugins are responsible for converting raw bytes containing message
data into actual Message struct objects that the Heka pipeline can process. As
with inputs and splitters, the ``Decoder`` interface is quite simple::

    type Decoder interface {
        Decode(pack *PipelinePack) (packs []*PipelinePack, err error)
    }

There are three additional optional interfaces a decoder might decide to
implement. The first provides the decoder access to its DecoderRunner object
when it is started::

    type WantsDecoderRunner interface {
        SetDecoderRunner(dr DecoderRunner)
    }

The second provides a notification to the decoder when the DecoderRunner is
exiting::

    type WantsDecoderRunnerShutdown interface {
        Shutdown()
    }

Understanding the third optional interface requires a bit of context. Heka's
PipelinePack structs contain a Message attribute, which points to the actual
instantiated Message struct, and a MsgBytes attribute, which is generally used
to hold the protocol buffer encoding of the Message struct. Whenever a message
is injected into the message router, Heka will protobuf encode that message
and store the result in the MsgBytes attribute, also setting the pack's
``TrustMsgBytes`` attribute flag to ``true``.

In some cases, however, a protobuf encoding of the message is already
available. For instance, when a message is received in protobuf format and is
not further mutated, as in the case when an input is using a single
ProtobufDecoder, then the original incoming data is already a valid protobuf
encoding. Any decoder that might already have access to or generate a valid
protobuf encoding for the resulting message should implement the
``EncodesMsgBytes`` interface::

    type EncodesMsgBytes interface {
        EncodesMsgBytes() bool
    }

Heka will check for this method at startup and, if it exists, it will assume
that the decoder plugin may populate the MsgBytes data with the encoded
message data, and that it will set pack.TrustMsgBytes to true if it does.

A decoder's ``Decode`` method should extract raw message data from the
provided pack. Depending on the nature of the decoder, this might be found
either in the MsgBytes attribute of the PipelinePack, or in the contained
Message struct's Payload field. Then it should try to deserialize and/or parse
this raw data, using the contained information to overwrite or populate the
pack's Message struct.

If the decoding / parsing operation concludes successfully then Decode should
return a slice of PipelinePack pointers and a nil error value. The first item
in the returned slice (i.e. ``packs[0]``) should be the pack that was passed
in to the method. If the decoding process needs to produce more than one
output pack, additional ones can be obtained from the DecoderRunner's
``NewPack`` method, and they should be appended to the returned slice of packs.

If decoding fails for any reason, then Decode should return a nil value for
the PipelinePack slice and an appropriate error value. Returning an error will
cause Heka to log an error message about the decoding failure. Additionally,
if the associated input plugin's configuration set the ``send_decode_failure``
value to true, the message will be tagged with ``decode_failure`` and
``decode_error`` fields and delivered to the router.

.. _no_mutate_post_router_warning:

About Message Mutation
======================

All of the above plugin types (i.e. inputs, splitters, and decoders) come
*before* the router in Heka's pipeline, and therefore they may safely mutate
the message struct. Once a pack hits the router, however, it is no longer safe
to mutate the message, because a) it might be concurrently processed by more
that one filter and/or output plugin, leading to race conditions, and b) a
protobuf encoding of the message will be stored in the pack.MsgBytes
attribute, and mutating the message will cause this encoding to become out of
sync with the actual message.

**Filter, encoder, and output plugins should never mutate Heka messages.**
Sandbox plugins will prevent you from doing so. SandboxEncoders, in
particular, expose the ``write_message`` API that appears to mutate a message,
but it actually creates a new message struct rather than modifying the
existing one (i.e. copy-on-write). If you implement your own filter, encoder,
or output plugins in Go, you must take care to honor this requirement and not
modify the message.

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

Like input plugins, filters have a ``Run`` method which accepts a runner and a
helper, and which should not return until shutdown unless there's an error
condition. The similarities end there, however.

Filters should call ``runner.InChan()`` to gain access to the plugin's input
channel. A filter's input channel provides pointers to PipelinePack objects,
defined in `pipeline_runner.go <https://github.com/mozilla-
services/heka/blob/master/pipeline/pipeline_runner.go>`_, each of which
contains what should be by now a fully populated Message struct from which the
filter can extract any desired information.

Upon processing a message, a filter plugin can perform any of three tasks:

1. Pass the original message through unchanged to one or more specific
   alternative Heka filter or output plugins.
2. Generate one or more *new* messages, which can be passed to either a
   specific set of Heka plugins, or which can be handed back to the router to
   be checked against all registered plugins' ``message_matcher`` rules.
3. Nothing (e.g. when performing counting / aggregation / roll-ups).

To pass a message through unchanged, a filter can call
``PluginHelper.Filter()`` or ``PluginHelper.Output()`` to access a filter or
output plugin, and then call that plugin's ``Deliver()`` method, passing in
the ``PipelinePack``.

To generate new messages, your filter must call
``PluginHelper.PipelinePack(msgLoopCount int)``. The ``msgloopCount`` value to
be passed in should be obtained from the ``MsgLoopCount`` value on the pack
that you're already holding, or possibly zero if the new message is being
triggered by a timed ticker instead of an incoming message. The PipelinePack
method will either return a pack ready for you to populate or nil if the loop
count is greater than the configured maximum value, as a safeguard against
inadvertently creating infinite message loops.

Once a pack has been obtained, a filter plugin can populate its Message
struct. The pack can then be passed along to a specific plugin (or plugins) as
above. Alternatively, the pack can be injected into the Heka message router
queue, where it will be checked against all plugin message matchers, by
passing it to the ``FilterRunner.Inject(pack *PipelinePack)`` method. Note
that, again as a precaution against message looping, a plugin will not be
allowed to inject a message which would get a positive response from that
plugin's own matcher.

Sometimes a filter will take a specific action triggered by a single incoming
message. There are many cases, however, when a filter is merely collecting or
aggregating data from the incoming messages, and instead will be sending out
reports on the data that has been collected at specific intervals. Heka has
built-in support for this use case. Any filter (or output) plugin can include
a ``ticker_interval`` config setting (in seconds, integers only), which will
automatically be extracted by Heka when the configuration is loaded. Then from
within your plugin code you can call ``FilterRunner.Ticker()`` and you will
get a channel (type ``<-chan time.Time``) that will send a tick at the
specified interval. Your plugin code can listen on the ticker channel and take
action as needed.

Observant readers might have noticed that, unlike the ``Input`` interface,
filters don't need to implement a ``Stop`` method. Instead, Heka will
communicate a shutdown event to filter plugins by closing the input channel
from which the filter is receiving PipelinePacks. When this channel is closed,
a filter should perform any necessary clean-up and then return from the Run
method with a nil value to indicate a clean exit.

Finally, there is one very important point that all authors of filter plugins
should keep in mind: if you are *not* passing your received PipelinePack
object on to another filter or output plugin for further processing, then you
*must* call ``pack.Recycle()`` to tell Heka that you are through with the
pack. Failure to do so will cause Heka to not free up the packs for reuse,
exhausting the supply and eventually causing the entire pipeline to freeze.

.. _encoders:

Encoders
========

Encoder plugins are the inverse of decoders. They convert Message structs into
raw bytes that can be delivered to the outside world. Some encoders will
serialize an entire Message struct, such as the :ref:`config_protobufencoder`
which uses Heka's native protocol buffers format. Other encoders extract data
from the message and insert it into a different format such as plain text or
JSON.

The ``Encoder`` interface consists of one method::

    type Encoder interface {
        Encode(pack *PipelinePack) (output []byte, err error)
    }


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
``use_framing`` is set to true in the output's configuration, however, the
OutputRunner will prepend Heka's :ref:`stream_framing` to the generated binary
data.

Outputs can also directly access their encoder instance by calling
OutputRunner.Encoder(). Encoders themselves don't handle the stream framing,
however, so it is recommended that outputs use the OutputRunner method
instead.

Even though encoders don't run in their own goroutines, it is possible that
they might need to perform some clean up at shutdown time. If this is so, the
encoder can implement the ``NeedsStopping`` interface::

    type NeedsStopping interface {
        Stop()
    }

And the ``Stop`` method will be called during the shutdown sequence.

.. _outputs:

Outputs
=======

Finally we come to the output plugins, which are responsible for receiving
Heka messages and using them to generate interactions with the outside world.
The ``Output`` interface is nearly identical to the ``Filter`` interface::

    type Output interface {
        Run(or OutputRunner, h PluginHelper) (err error)
    }

In fact, there are many ways in which filter and output plugins are similar.
Like filters, outputs should call the ``InChan`` method on the provided runner
to get an input channel, which will feed PipelinePacks. Like filters, outputs
should listen on this channel until it is closed, at which time they should
perform any necessary clean-up and then return. And, like filters, any output
plugin with a ``ticker_interval`` value in the configuration will use that
value to create a ticker channel that can be accessed using the runner's
``Ticker`` method. And, finally, outputs should also be sure to call
``pack.Recycle()`` when they finish w/ a pack so that Heka knows the pack is
freed up for reuse.

The primary way that outputs differ from filters, of course, is that outputs
need to serialize (or extract data from) the messages they receive and then
send that data to an external destination. The serialization (or data
extraction) should typically be performed by the output's specified encoder
plugin. The OutputRunner exposes the following methods to assist with this::

    Encode(pack *PipelinePack) (output []byte, err error)
    UsesFraming() bool
    Encoder() (encoder Encoder)

The ``Encode`` method will use the specified encoder to convert the pack's
message to binary data, then if ``use_framing`` was set to true in the
output's configuration it will prepend Heka's :ref:`stream_framing`. The
``UsesFraming`` method will tell you whether or not ``use_framing`` was set to
true. Finally, the ``Encoder`` method will return the actual encoder that was
registered. This is useful to check to make sure that an encoder was actually
registered, but generally you will want to use OutputRunner.Encode and not
Encoder.Encode, since the latter will not honor the output's ``use_framing``
specification.

.. _register_custom_plugins:

Registering Your Plugin
=======================

The last step you have to take after implementing your plugin is to register
it with Heka so it can actually be configured and used. You do this by calling
the ``pipeline`` package's ``RegisterPlugin`` function::

    func RegisterPlugin(name string, factory func() interface{})

The ``name`` value should be a unique identifier for your plugin, and it
should end in one of "Input", "Splitter", "Decoder", "Filter", "Encoder", or
"Output", depending on the plugin type.

The ``factory`` value should be a function that returns an instance of your
plugin, usually a pointer to a struct, where the pointer type implements the
``Plugin`` interface and the interface appropriate to its type (i.e.
``Input``, ``Splitter``, ``Decoder``, etc).

This sounds more complicated than it is. Here are some examples from Heka
itself::

    RegisterPlugin("UdpInput", func() interface{} {return new(UdpInput)})
    RegisterPlugin("TcpInput", func() interface{} {return new(TcpInput)})
    RegisterPlugin("ProtobufDecoder", func() interface{} {return new(ProtobufDecoder)})
    RegisterPlugin("CounterFilter", func() interface{} {return new(CounterFilter)})
    RegisterPlugin("StatFilter", func() interface{} {return new(StatFilter)})
    RegisterPlugin("LogOutput", func() interface{} {return new(LogOutput)})
    RegisterPlugin("FileOutput", func() interface{} {return new(FileOutput)})

It is recommended that ``RegisterPlugin`` calls be put in your Go package's
`init() function <http://golang.org/doc/effective_go.html#init>`_ so that you
can simply import your package when building ``hekad`` and the package's
plugins will be registered and available for use in your Heka config file.
This is made a bit easier if you use ``plugin_loader.cmake``, see
:ref:`build_include_externals`.

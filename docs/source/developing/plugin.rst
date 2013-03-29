.. _plugins:

==============
Extending Heka
==============

The core of the Heka engine is written in the `Go <http://golang.org>`_
programming language. Heka supports plugins, which are also written in
Go. This document will try to provide enough information for developers
to extend Heka by implementing their own custom plugins. It assumes a
small amount of familiarity with Go, although any reasonably
experienced programmer will probably be able to follow along with no
trouble.

.. _extending_definitions:

Definitions
===========

You should be familiar with the :ref:`glossary` terminology before
proceeding.

.. _plugin_config:

Plugin Configuration
====================

Heka uses JSON as its configuration file format (see:
:ref:`configuration`), and provides a simple mechanism through which
plugins can integrate with the configuration loading system to
initialize themselves from settings in the config file.

The minimal shared interface that a Heka plugin must implement in order to use
the config system is (surprise, surprise) `Plugin`, defined in
`pipeline_runner.go <https://github.com /mozilla-
services/heka/blob/dev/pipeline/pipeline_runner.go>`_::

    type Plugin interface {
            Init(config interface{}) error
    }

During Heka initialization a pool of instances of every plugin listed in the
configuration file will be created. The JSON configuration for each plugin
will be parsed and the result will be passed in to the `Init` method of each
instance as the `config` argument. This argument is specified as type
`interface{}`, but by default the underlying type will be
`*pipeline.PluginConfig`, which is just an alias for `map[string]interface{}`,
providing the config data as simple key/value pairs. There is also a way for
plugins to specify a custom struct to be used instead of the generic
`PluginConfig` type (see :ref:`custom_plugin_config`). In either event, the
plugin instances can use the config file values that have been loaded into the
provided `config` object to perform required initialization.

As an example, imagine you were writing a plugin that would only affect
messages that were from a specific set of hosts. The hosts could be specified
in the config as a JSON array of hostnames, and your plugin definition and
`Init` method might look like this::

    type MyPlugin struct {
            hosts []string
    }

    // Extract 'hosts' value from config and store it on the plugin instance
    func (self *MyPlugin) Init(config interface{}) error {
            conf := config.(*pipeline.PluginConfig)
            hostsConf, ok := conf['hosts']
            if !ok {
                    return errors.New("MyPlugin: No 'hosts' setting configured.")
            }
            hostsSeq, ok := hostsConf.([]interface)
            if !ok {
                    return errors.New("MyPlugin: 'hosts' setting not a sequence.")
            }
            self.hosts = make([]string, 0, len(hostsSeq))
            for _, host := range(hostsSeq) {
                    if hostStr, ok := host.(string); ok {
                            self.hosts = append(self.hosts, hostStr)
                    } else {
                            return fmt.Errorf("MyPlugin: hostname not a string: %+v", host)
                    }
            }
            return nil
    }

If your plugin is going to require a global object shared among all of the
plugin instances in the pool then instead of `Plugin` you should provide the
closely related `PluginWithGlobal` interface, also defined in
`pipeline_runner.go <https://github.com/mozilla-
services/heka/blob/dev/pipeline/pipeline_runner.go>`_.::

    type PluginWithGlobal interface {
            Init(global PluginGlobal, config interface{}) error
            InitOnce(config interface{}) (global PluginGlobal, err error)
    }

When Heka loads configuration for a `PluginWithGlobal` type from the config
file, it will first create an instance of the plugin and then call `InitOnce`,
passing in the loaded config data. `InitOnce` should perform any one-time-only
initialization (opening an outgoing network connection, for example) and then
create and return a custom `PluginGlobal` object containing any resources that
will need to be shared among the plugin pool. The global objects can be of
any type that support the `PluginGlobal` interface::

    type PluginGlobal interface {
            // Called when an event occurs, either RELOAD or STOP
            Event(eventType string)
    }

After the `PluginGlobal` is returned from `InitOnce`, Heka will create the
pool of `PluginWithGlobal` instances, calling `Init` on each one and passing
in both the PluginGlobal *and* the config object.

Consider an output plugin that will send data out over a UDP connection. The
initialization code might look like so::

    // This will be our pipeline.PluginGlobal type
    type UdpOutputGlobal struct {
            conn net.Conn
    }

    // Provides pipeline.PluginGlobal interface
    func (self *UdpOutputGlobal) Event(eventType string) {
            if eventType == pipeline.STOP {
                    self.conn.Close()
            }
    }

    // This will be our PluginWithGlobal type
    type UdpOutput struct {
            global *UdpOutputGlobal
    }

    // Initialize UDP connection, store it on the PluginGlobal
    func (self *UdpOutput) InitOnce(config interface{}) (pipeline.PluginGlobal, error) {
            conf := config.(*pipeline.PluginConfig)
            addr, ok := conf["address"]
            if !ok {
                    return nil, errors.New("UdpOutput: No UDP address")
            }
            addrStr, ok := addr.(string)
            if !ok {
                    return nil, errors.New("UdpOutput: UDP address not a string")
            }
            udpAddr, err := net.ResolveUdpAddr("udp", addr)
            if err != nil {
                    return nil, fmt.Errorf("UdpOutput error resolving UDP address %s: %s",
                            addrStr, err.Error())
            }
            udpConn, err := net.DialUDP("udp", nil, udpAddr)
            if err != nil {
                    return nil, fmt.Errorf("UdpOutput error dialing UDP address %s: %s",
                            addrStr, err.Error())
            }
            return &UdpOutputGlobal{udpConn}, nil
    }

    // Store a reference to the global for use during pipeline processing
    func (self *UdpOutput) Init(global pipeline.PluginGlobal, config interface{}) error {
            self.global = global // UDP connection available as self.global.conn
            return nil
    }

.. _custom_plugin_config:

Custom Plugin Config Structs
============================

In simple cases it might be sufficient to receive plugin configuration data as
a generic map of keys and values, but if there are more than a couple of
config settings then checking for, extracting, and validating the values
quickly becomes unwieldy. Heka supports a rudimentary plugin configuration
schema system by making use of the Go language's automatic parsing of JSON
values into suitable struct objects.

Plugins that wish to provide a custom configuration struct to be populated
from the config file JSON should implement the `HasConfigStruct` interface
defined in the `config.go <https://github.com/mozilla-
services/heka/blob/dev/pipeline/config.go>`_ file::

    type HasConfigStruct interface {
            ConfigStruct() interface{}
    }

Your code should define a struct that can hold the required config values, and
you should then implement a `ConfigStruct` method on your plugin which will
initialize one of these and return it. Heka's config loader will then use this
object as the value to be populated when Go's `json.Unmarshal` is called with
the corresponding JSON from the config file. Note that this also gives you a
mechanism for specifying default config values, by populating your config
struct as desired before returning it from the `ConfigStruct` method.

Revisiting our example above, let's say we wanted to have our `UdpOutput`
plugin default to sending data to my.example.com, port 44444. The
initialization code might look as follows::

    // This will be our pipeline.PluginGlobal type
    type UdpOutputGlobal struct {
            conn net.Conn
    }

    // Provides pipeline.PluginGlobal interface
    func (self *UdpOutputGlobal) Event(eventType string) {
            if eventType == pipeline.STOP {
                    self.conn.Close()
            }
    }

    // This will be our PluginWithGlobal type
    type UdpOutput struct {
            global *UdpOutputGlobal
    }

    // This is our plugin's custom config struct
    type UdpOutputConfig struct {
            Address string
    }

    // Provides pipeline.HasConfigStruct interface, populates default value
    func (self *UdpOutput) ConfigStruct() interface{} {
            return &UdpOutputConfig{"my.example.com:44444"}
    }

    // Initialize UDP connection, store it on the PluginGlobal
    func (self *UdpOutput) InitOnce(config interface{}) (pipeline.PluginGlobal, error) {
            conf := config.(*UdpOutputConfig) // assert we have the right config struct type
            udpAddr, err := net.ResolveUdpAddr("udp", conf.Address)
            if err != nil {
                    return nil, fmt.Errorf("UdpOutput error resolving UDP address %s: %s",
                            conf.Address, err.Error())
            }
            udpConn, err := net.DialUDP("udp", nil, udpAddr)
            if err != nil {
                    return nil, fmt.Errorf("UdpOutput error dialing UDP address %s: %s",
                            conf.Address, err.Error())
            }
            return &UdpOutputGlobal{udpConn}, nil
    }

    // Store a reference to the global for use during pipeline processing
    func (self *UdpOutput) Init(global pipeline.PluginGlobal, config interface{}) error {
            self.global = global // UDP connection available as self.global.conn
            return nil
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

.. _runner_plugin:

Runner Plugin
=============

The `Runner` plugin is a special plugin Heka provides that efficiently manages
writing to a shared connection. To make use of the `Runner` plugin you must
provide a `Writer` object that knows how to prepare data for output and to
perform the actual write operation, or a `BatchWriter` if you want to queue up
output and send it out in batches. `Writer` and `BatchWriter` are defined (in
`runner_plugin.go <https://github.com /mozilla-
services/heka/blob/dev/pipeline/runner_plugin.go>`_) as follows::

    type Writer interface {
            PluginGlobal
            DataRecycler

            Init(config interface{}) error
            Write(outData interface{}) error
    }

    type BatchWriter interface {
            PluginGlobal
            DataRecycler

            Init(config interface{}) (<-chan time.Time, error)
            Batch(outData interface{}) error
            Commit() error
    }

You'll note that each of these embed both the `PluginGlobal` and
`DataRecycler` interfaces, which together specify four methods::

    type PluginGlobal interface {
            Event(eventType string)
    }

    type DataRecycler interface {
            MakeOutData() interface{}
            ZeroOutData(outData interface{})
            PrepOutData(pack *PipelinePack, outData interface{}, timeout *time.Duration) error
    }

So a `Writer` must provide a total six methods while a `BatchWriter` must
provide seven. Following is a more detailed look at each of these interfaces
and the methods you must implement.

.. _pluginglobal_interface:

PluginGlobal Interface
----------------------

The writer object you implement will actually serve as the "global" object for
a particular pool of `Runner` plugins, so it must provide the `PluginGlobal`
interface to wire it up to Heka's configuration and event notification
systems. `PluginGlobal` is a single method:

Event(eventType string)
    The `Event` method ties in to Heka's event notification system.
    `eventType` will be one of two constants: `pipeline.STOP` or
    `pipeline.RELOAD`. Your writer should check to see which event was passed
    and perform any resource shutdown or reloading as appropriate.

.. _datarecycler_interface:

DataRecycler Interface
----------------------

While all of the information that is to be sent out is usually embedded within
the message object, it needs to be extracted and packaged up before it can be
sent over the wire. Heka writers must provide an `outData` object, into which
extracted message data can be placed. The `Runner` plugin doesn't care what
type the `outData` is, but it **must** be a pointer of some sort so it can be
modified by methods to which it is passed. (Note that this is true even if
`outData` is a reference object such as a slice or a map.)

The `DataRecycler` interface defines the methods related to creating,
preparing, and recycling these `outData` objects:

MakeOutData() (outData interface{})
    Despite the name, this method will not provide you with information about
    who has been kissing whom among your circle of friends. Instead, this
    method on your writer object is responsible for instantiating and
    returning exactly one `outData` pointer object which will be in use for
    the life of the Heka process.

ZeroOutData(outData interface{})
    After an `outData` object has been used and its contents have been sent on
    to their ultimate destination, it will be recycled. `ZeroOutData` will be
    passed a used `outData` object to be reset to a zero condition so it is
    suitable for reuse.

PrepOutData(pack *PipelinePack, outData interface{}, timeout *time.Duration) error
    This is the method that performs the real work. It will be passed a
    `*PipelinePack` object (containing a populated `Message` object) and a
    zeroed `outData` object. `PrepOutData` must extract any desired data from
    the `PipelinePack` and populate `outData` for delivery. (The `timeout`
    argument you can ignore for now. It will always be `nil` unless you are
    using the `Runner` plugin as an input.)

It is important to realize that all of the `DataRecycler` methods will be
simultaneously in use by the entire pool of Heka pipelines, so they must be
thread-safe. `ZeroOutData` and `PrepOutData` can (and should) modify the
passed `outData` pointer object, but they should **not** try to assume
ownership of writer attributes or any other resource that is also available
to other goroutines.

.. _writer_interface:

Writer Interface
----------------

Once a `DataRecycler` implementation has set up management of our `outData`
objects, we can get to the task of actually writing to the output by providing
a `Writer` implementation:

Init(config interface{}) error
    This is a setup method that will be called exactly once. This is wired up
    to Heka's config system, and any configuration values specified for this
    particular `Runner` plugin will be passed along to the `Writer`. As with
    plugins, `config` will be of type `PluginConfig` (i.e.
    `map[string]interface{}`) by default, but you can instead specify a custom
    config struct by implementing a `ConfigStruct` method to satisfy the
    `HasConfigStruct` interface (see :ref:`custom_plugin_config`). The `Init`
    method is where you would open a file handle, establish a persistent
    network connection, or do any other initialization of resources to be
    shared by the entire pool of pipelines.

Write(outData interface{}) error
    The `Write` method receives a populated `outData` object and is
    responsible for sending the data out over the wire. It will be called
    repeatedly, but for a given `Writer` instance it will only ever be called
    from a single goroutine, so it is safe to make use of any shared resource
    without needing to worry about contention or locks.

.. _batchwriter_interface:

BatchWriter Interface
---------------------

`BatchWriter` is very like `Writer`, except that there's a two-stage write
process where messages are added to a batch in one step and then an entire
batch is written out in another.

Init(config interface{}) (<-chan time.Time, error)
    This `Init` method will be called exactly once, and is nearly identical in
    signature and functionality to its namesake in the `Writer` interface,
    above. The only difference is that this version must also return a
    "ticker" channel that will signal to the `Runner` plugin when a batch of
    accumulated data should be written. This channel is specified to carry
    `time.Time` objects so you can trivially use the channels returned by Go's
    `time.Tick <http://golang.org/pkg/time/#Tick>`_ function.

Batch(outData interface{}) error
    The `Batch` method will be called for each message that is to be
    delivered, and works much like the `Write` method above in that it a) will
    be passed a populated `outData` object and b) will only be called from one
    goroutine at a time. Unlike `Write`, however, `Batch` should not actually
    send data out over the wire. Instead, it should place the data into a
    buffer of some sort for holding until the next tick triggers a write
    operation.

Commit() error
    `Commit` is the method that will be called when a tick is delivered over
    the ticker channel returned by the `Init` method, and is responsible for
    grabbing all of the data that the `Batch` method has accumulated and
    writing it out. `Commit` will only ever be called from a single goroutine,
    and in fact it will never be called at the same time as `Batch`, so it is
    safe to pop all of the accumulated data from the delivery buffer without
    worrying about locking or race conditions.

BatchWriter Example
===================

To put this together, let's reconsider the `UdpOutput` we were working on
above, where we want to write a message's payload out over a UDP connection.
Only we'll extend this to accumulate messages in batches and only actually
send a batch out once every second.

All we need is a `UdpBatchWriter` implementation and a corresponding
`UdpBatchWriterConfig` struct to hold the configuration data::

    type UdpBatchWriterConfig struct {
            Address string
    }

    type UdpBatchWriter struct {
            conn net.Conn
            batchBuffer []*[]byte
    }

    // Provides pipeline.HasConfigStruct interface
    func (self *UdpBatchWriter) ConfigStruct interface{} {
            return &UdpBatchWriterConfig{"my.example.com:44444"}
    }

    // Provides pipeline.PluginGlobal interface
    func (self *UdpBatchWriter) Event(eventType string) {
            if eventType == pipeline.STOP {
                    self.conn.Close()
            }
    }

    // Sets up UDP connection, buffer for accumulating output data, and commit ticker
    func (self *UdpBatchWriter) Init(config interface{}) (<-chan time.Time, error) {
            conf := config.(*UdpBatchWriterConfig)
            udpAddr, err := net.ResolveUdpAddr("udp", conf.Address)
            if err != nil {
                    return nil, fmt.Errorf("UdpBatchWriter error resolving UDP address %s: %s",
                            conf.Address, err.Error())
            }
            udpConn, err := net.DialUDP("udp", nil, udpAddr)
            if err != nil {
                    return nil, fmt.Errorf("UdpBatchWriter error dialing UDP address %s: %s",
                            conf.Address, err.Error())
            }
            self.batchBuffer = make([]*[]byte, 0, pipeline.PoolSize*2)
            return time.Tick(time.Second), nil
    }

    // Creates a byte slice for holding output data
    func (self *UdpBatchWriter) MakeOutData() interface{} {
            b := make([]byte, 0, 1000)
            return &b
    }

    // Resets a byte slice to zero length for reuse
    func (self *UdpBatchWriter) ZeroOutData(outData interface{}) {
            outBytesPtr := outData.(*[]byte)
            *outBytesPtr = *outBytesPtr[:0]
    }

    // Writes message payload into provided outData byte slice
    func (self *UdpBatchWriter) PrepOutData(pack *pipeline.PipelinePack, outData interface{},
            timeout *time.Duration) error {
            outBytesPtr := outData.(*[]byte)
            *outBytesPtr = append(*outBytesPtr, []byte(pack.Message.Payload)...)
            return nil
    }

    // Adds this particular outData byte slice into the output buffer
    func (self *UdpBatchWriter) Batch(outData interface{}) error {
            outBytesPtr := outData(*[]byte)
            self.batchBuffer = append(self.batchBuffer, outBytesPtr)
            return nil
    }

    // Extract all of the outgoing message contents from the output buffer, separate
    // them by newlines, and write the result out over the UDP connection
    func (self *UdpBatchWriter) Commit() error {
            fullBatch := make([]byte, 0, 2000)
            for _, outBytesPtr := range(self.batchBuffer) {
                    fullBatch = append(fullBatch, (*outBytesPtr)...)
                    fullBatch = append(fullBatch, []byte("\n"))
            }
            n, err := self.conn.Write(fullBatch)
            if err != nil {
                    return fmt.Errorf("UdpBatchWriter commit error: %s", err.Error())
            }
            if n < len(fullBatch) {
                    return errors.New("UdpBatchWriter commit write truncated")
            }
            return nil
    }

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

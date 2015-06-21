====
Heka
====

.. start-description

Heka is an open source stream processing software system developed by `Mozilla
<https://mozilla.org>`_. Heka is a "Swiss Army Knife" type tool for data
processing, useful for a wide variety of different tasks, such as:

* Loading and parsing log files from a file system.

* Accepting `statsd <https://github.com/etsy/statsd/>`_ type metrics data for
  aggregation and forwarding to upstream time series data stores such as
  `graphite <http://graphite.wikidot.com/>`_ or `InfluxDB
  <http://influxdb.org/>`_.

* Launching external processes to gather operational data from the local
  system.

* Performing real time analysis, graphing, and anomaly detection on any data
  flowing through the Heka pipeline.

* Shipping data from one location to another via the use of an external
  transport (such as AMQP) or directly (via TCP).

* Delivering processed data to one or more persistent data stores.

The following resources are available to those who would like to ask
questions, report problems, or learn more:

* Mailing List: https://mail.mozilla.org/listinfo/heka
* Issue Tracker: https://github.com/mozilla-services/heka/issues
* Github Project: https://github.com/mozilla-services/heka/
* IRC: #heka channel on irc.mozilla.org

Heka is a heavily plugin based system. Common operations such as adding data
to Heka, processing it, and writing it out are implemented as plugins. Heka
ships with numerous plugins for performing common tasks.

There are six different types of Heka plugins:

:ref:`config_inputs`

  Input plugins acquire data from the outside world and inject it into the
  Heka pipeline. They can do this by reading files from a file system,
  actively making network connections to acquire data from remote servers,
  listening on a network socket for external actors to push data in, launching
  processes on the local system to gather arbitrary data, or any other
  mechanism.

  Input plugins must be written in Go.

:ref:`config_splitters`

  Splitter plugins receive the data that is being acquired by an input plugin
  and slice it up into individual records. They must be written in Go.

:ref:`config_decoders`

  Decoder plugins convert data that comes in through the Input plugins to
  Heka's internal Message data structure. Typically decoders are responsible
  for any parsing, deserializing, or extracting of structure from unstructured
  data that needs to happen.

  Decoder plugins can be written entirely in Go, or the core logic can
  be written in sandboxed Lua code.

:ref:`config_filters`

  Filter plugins are Heka's processing engines. They are configured to receive
  messages matching certain specific characteristics (using Heka's
  :ref:`message_matcher`) and are able to perform arbitrary monitoring,
  aggregation, and/or processing of the data. Filters are also able to
  generate new messages that can be reinjected into the Heka pipeline, such as
  summary messages containing aggregate data, notification messages in cases
  where suspicious anomalies are detected, or circular buffer data messages
  that will show up as real time graphs in Heka's dashboard.

  Filters can be written entirely in Go, or the core logic can be written in
  sandboxed Lua code. It is also possible to configure Heka to allow Lua
  filters to be dynamically injected into a running Heka instance without
  needing to reconfigure or restart the Heka process, nor even to have
  shell access to the server on which Heka is running.

:ref:`config_encoders`

  Encoder plugins are the inverse of Decoders. They generate arbitrary byte
  streams using data extracted from Heka Message structs. Encoders are
  embedded within Output plugins; Encoders handle the serialization, Outputs
  handle the details of interacting with the outside world.

  Encoder plugins can be written entirely in Go, or the core logic can
  be written in sandboxed Lua code.

:ref:`config_outputs`

  Output plugins send data that has been serialized by an Encoder to some
  external destination. They handle all of the details of interacting with the
  network, filesystem, or any other outside resource. They are, like Filters,
  configured using Heka's :ref:`message_matcher` so they will only receive and
  deliver messages matching certain characteristics.

  Output plugins must be written in Go.

Information about developing plugins in Go can be found in the :ref:`plugins`
section. Details about using Lua sandboxes for Decoder, Filter, and Encoder
plugins can be found in the :ref:`sandbox` section.

.. end-description

.. start-hekad

hekad
=====

The core of the Heka system is the `hekad` daemon. A single `hekad` process
can be configured with any number of plugins, simultaneously performing a
variety of data gathering, processing, and shipping tasks. Details on how to
configure a `hekad` daemon are in the :ref:`configuration` section.

hekad Command Line Options
==========================

.. start-options

``-version``
    Output the version number, then exit.

``-config`` `config_path`
    Specify the configuration file or directory to use; the default is
    /etc/hekad.toml. If `config_path` resolves to a directory, all files in
    that directory must be valid TOML files. (See hekad.config(5).)

.. end-options

.. end-hekad

Contents:

.. toctree::
   :maxdepth: 3

   installing
   getting_started
   config/index
   config/inputs/index
   config/splitters/index
   config/decoders/index
   config/filters/index
   config/encoders/index
   config/outputs/index
   monitoring/index
   developing/plugin
   message/index
   message_matcher
   sandbox/index
   developing/testing
   tls

.. toctree::
   :hidden:

   buffering
   changelog
   developing/old_apis
   glossary
   sandbox/graph_annotation
   sandbox/json_payload_transform
   config/common_sandbox_parameter
   pluginconfig/logstreamer
   man/config
   man/plugin
   man/usage


Indices and tables
==================

* :ref:`search`
* :ref:`glossary`
* :ref:`changelog`

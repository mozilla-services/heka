.. _glossary:

Glossary
========

.. glossary::
    :sorted:

    hekad
        Daemon that routes messages from inputs to their outputs applying
        filters as configured.

    Message
        A message is the atomic unit of data that Hekad deals with. It
        is a data structure related to a single event happening in the
        outside world, such as a log file entry, a counter increment,
        an application exception, a notification message, etc. It is
        specified as a `Message` struct in the `heka/message` packages
        `message.go <https://github.com/mozilla-
        services/heka/blob/master/message/message.go>`_ file.

    Message matcher
        A configuration option for filter and output plugins that specifies
        which messages that plugin accepts for processing. The Heka router
        will evaluate the message matchers against every message to and will
        deliver the message when the match is positive.

    Pipeline
        Messages being processed by Hekad are passed through a specific set of
        plugins. A set of plugins to be applied to a message is often called
        (somewhat informally) a Heka pipeline.

    PipelinePack
        In addition to the core message data, Hekad needs to track some
        related state and configuration information for each message. To this
        end there is a `PipelinePack` struct defined in the `heka/pipeline`
        package's `pipeline_runner.go <https://github.com/mozilla-
        services/heka/blob/master/pipeline/pipeline_runner.go>`_ file.
        `PipelinePack` objects are what get passed in to the various Hekad
        plugins as messages flow through the pipelines.

    Plugin
        Hekad plugins are functional units that perform specific actions on or
        with messages. There are six types of plugins: inputs, splitters, 
        decoders, filters, encoders, and ouputs.

    PluginChanSize
        A Heka configuration setting which specifies the size of the input
        channel buffer for the various Heka plugins. Defaults to 50.

    PluginHelper
        An interface that provides access to certain Heka internals that may
        be required by plugins in the course of their activity. Defined in
        `config.go <https://github.com/mozilla-
        services/heka/blob/master/pipeline/config.go>`_.

    PluginRunner
        A plugin-specific helper object that manages the lifespan of a given
        plugin and handles most details of interaction with the greater Heka
        environment. Comes in five variants, each tailored to a specific
        plugin type: `InputRunner`, `SplitterRunner`, `DecoderRunner`, 
        `FilterRunner`, and `OutputRunner`.

    PoolSize
        A Heka configuration setting which specifies the number of
        `PipelinePack` structs that will be created. This value specifies the
        maximum number of incoming messages that Heka can be processing at any
        one time.

    Router
        Component in the Heka pipeline that accepts messages and delivers them
        to the appropriate filter and output plugins, as specified by the
        plugins' message matcher values.

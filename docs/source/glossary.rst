.. _glossary:

Glossary
========

.. glossary::
    :sorted:

    Message
        A message is the atomic unit of data that Hekad deals with. It
        is a data structure related to a single event happening in the
        outside world, such as a log file entry, a counter increment,
        an application exception, a notification message, etc. It is
        specified as a `Message` struct in the `heka/message` packages
        `message.go <https://github.com/mozilla-
        services/heka/tree/dev/message/message.go>`_ file.

    Plugin
        Hekad plugins are functional units that perform specific actions
        on or with messages. There are four distinct types of plugins:
        inputs, decoders, filters, and outputs.

    Pipeline
        Messages being processed by Hekad are passed through a specific
        set of plugins. A set of plugins to be applied to a message
        make up what is called a Hekad pipeline. Many Hekad pipelines can
        be processed simultaneously, each running in its own goroutine.

    PoolSize
        `PoolSize` is a Hekad configuration setting which specifies the
        maximum number of pipelines allowed to be concurrently processed in a
        running Hekad server. (default: 1000)

    PipelinePack

        In addition to the core message data, Hekad needs to track some
        related state and configuration information for each message.
        To this end there is a `PipelinePack` struct defined in the
        `heka/pipeline` package's `pipeline_runner.go
        <https://github.com/mozilla-
        services/heka/tree/dev/pipeline/pipeline_runner.go>`_ file.
        `PipelinePack` objects are what get passed in to the various
        Hekad plugins as messages flow through the pipelines.

    PluginWithGlobal / PluginGlobal
        When Hekad starts up, `PoolSize` copies of each decoder, filter,
        and output plugin are created and are stored in `PipelinePack`
        objects. Many plugins, however, need to share access to a
        single, unique resource among the entire pool of plugin
        instances; it would not be a great idea to have 1000 different
        copies of a file output all competing to open a handle to the
        same output file, for instance. For this reason, many plugins
        are of type `PluginWithGlobal` and have an accompanying
        `PluginGlobal` object (see `pipeline_runner.go
        <https://github.com/mozilla-
        services/heka/tree/dev/pipeline/pipeline_runner.go>`_). Each
        `PluginWithGlobal` plugin type specified in your Hekad
        configuration will cause `PoolSize` instances of the plugin
        itself to be created but only a single `PluginGlobal` instance
        to be shared between them.

.. _hekad_cli:

=====
hekad
=====

Synopsis
========

hekad [``-version``] [``-config`` `config_file`] [``-cpuprof`` `output_file`] [``-maxprocs`` `int`] [``-memprof`` `output_file`] [``-poolsize`` `int`]

Description
===========

.. start-description

The hekad daemon is the core component of the heka project, which
handles routing messages, generating metrics, aggregating statsd-type
messages, running plugins on the messages, and sending messages to the
configured destinations.

.. end-description

Options
=======

.. start-options

``-version``
    Output the version number, then exit.

``-config`` `config_file`
    Specify the configuration file to use; the default is /etc/hekad.json.  (See hekad.config(5).)

``-cpuprof`` `output_file`
    Turn on CPU profiling of hekad; output is logged to the `output_file`.

``-maxprocs`` `int`
    Enable multi-core usage; the default is 1 core. More cores will generally
    increase message throughput. Best performance is usually attained by
    setting this to 2 x (number of cores). This assumes each core is
    hyper-threaded.

``-memprof`` `output_file`
    Enable memory profiling; output is logged to the `output_file`.

``-poolsize`` `int`
    Toggle the pool size of maximum messages that can exist; default is 1000
    which is usually sufficient and performs optimally.

.. end-options

Files
=====

/etc/hekad.json     configuration file

See Also
========

hekad.config(5), hekad.plugin(5)

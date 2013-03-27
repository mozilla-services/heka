=====
hekad
=====

Synopsis
========

hekad [``-version``] [``-config`` `config_file`] [``-cpuprof`` `output_file`] [``-maxprocs`` `int`] [``-memprof`` `output_file`] [``-poolsize`` `int`]

Description
===========

Party like its hot.

Options
=======

``-version``
    Output the version number, then exit.

``-config`` `config_file`
    Specify the configuration file to use; the default is /etc/hekad.json.  (See hekad.config(5).)

``-cpuprof`` `output_file`
    Turn on CPU profiling of hekad; output is logged to the `output_file`.

``-maxprocs`` `int`
    Enable multi-core usage; the default is 1 core. More cores will generally
    increase message throughput.

``-memprof`` `output_file`
    Enable memory profiling; output is logged to the `output_file`.

``-poolsize`` `int`
    Toggle the pool size of maximum messages that can exist; default is 1000.

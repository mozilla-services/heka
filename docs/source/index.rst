=====
hekad
=====

.. start-description

The hekad daemon is the core component of the heka project, which handles
routing messages, generating metrics, aggregating statsd-type messages,
running plugins on the messages, and sending messages to the configured
destinations.

.. end-description

Command Line Options
====================

.. start-options

``-version``
    Output the version number, then exit.

``-config`` `config_path`
    Specify the configuration file or directory to use; the default is
    /etc/hekad.toml. If `config_path` resolves to a directory, all files in
    that directory must be valid TOML files. (See hekad.config(5).)

.. end-options

.. seealso::
    `heka project`_

Contents:

.. toctree::
   :maxdepth: 2

   installing
   config/index
   config/inputs/index
   config/decoders/index
   config/filters/index
   config/encoders/index
   config/outputs/index
   monitoring/index
   developing/plugin
   message/index
   message_matcher.rst
   sandbox/index
   developing/testing
   tls



Indices and tables
==================

* :ref:`search`
* :ref:`glossary`

.. _heka project: http://heka-docs.readthedocs.org

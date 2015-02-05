.. _config_outputs:

=======
Outputs
=======

.. _config_common_output_parameters:

Common Output Parameters
========================

There are some configuration options that are universally available to all
Heka output plugins. These will be consumed by Heka itself when Heka
initializes the plugin and do not need to be handled by the plugin-specific
initialization code.

- message_matcher (string, optional):
    Boolean expression, when evaluated to true passes the message to the
    filter for processing. Defaults to matching nothing. See:
    :ref:`message_matcher`
- message_signer (string, optional):
    The name of the message signer. If specified only messages with this
    signer are passed to the filter for processing.
- ticker_interval (uint, optional):
    Frequency (in seconds) that a timer event will be sent to the filter.
    Defaults to not sending timer events.
- encoder (string, optional):
    .. versionadded:: 0.6

    Encoder to be used by the output. This should refer to the name of an
    encoder plugin section that is specified elsewhere in the TOML
    configuration. Messages can be encoded using the specified encoder by
    calling the OutputRunner's `Encode()` method.
- use_framing (bool, optional):
    .. versionadded:: 0.6

    Specifies whether or not Heka's :ref:`stream_framing` should be applied to
    the binary data returned from the OutputRunner's `Encode()` method.
- can_exit (bool, optional)
    .. versionadded:: 0.7
    
    Whether or not this plugin can exit without causing Heka to shutdown.
    Defaults to false.

Available Output Plugins
========================

.. toctree::
   :maxdepth: 1

   amqp
   carbon
   dashboard
   elasticsearch
   file
   http
   irc
   kafka
   log
   nagios
   smtp
   tcp
   udp
   whisper

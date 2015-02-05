.. _config_statstofieldsdecoder:

Stats To Fields Decoder
=======================

.. versionadded:: 0.4

Plugin Name: **StatsToFieldsDecoder**

The StatsToFieldsDecoder will parse time series statistics data in the
`graphite message format <http://graphite.wikidot.com/getting-your-data-into-
graphite#toc4>`_ and encode the data into the message fields, in the same
format produced by a :ref:`config_stat_accum_input` plugin with the
`emit_in_fields` value set to true. This is useful if you have externally
generated graphite string data flowing through Heka that you'd like to process
without having to roll your own string parsing code.

This decoder has no configuration options. It simply expects to be passed
messages with statsd string data in the payload. Incorrect or malformed
content will cause a decoding error, dropping the message.

The fields format only contains a single "timestamp" field, so any payloads
containing multiple timestamps will end up generating a separate message for
each timestamp. Extra messages will be a copy of the original message except
a) the payload will be empty and b) the unique timestamp and related stats
will be the only message fields.

Example:

.. code-block:: ini

	[StatsToFieldsDecoder]

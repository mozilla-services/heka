.. _config_inputs:

======
Inputs
======

.. _config_common_input_parameters:

Common Input Parameters
=======================

.. versionadded:: 0.9

There are some configuration options that are universally available to all
Heka input plugins. These will be consumed by Heka itself when Heka
initializes the plugin and do not need to be handled by the plugin-specific
initialization code.

- decoder (string, optional):
	Decoder to be used by the input. This should refer to the name of a
	registered decoder plugin configuration. If supplied, messages will be
	decoded before being passed on to the router when the InputRunner's
	`Deliver` method is called.
- synchronous_decode (bool, optional):
	If `synchronous_decode` is false, then any specified decoder plugin will
	be run by a DecoderRunner in its own goroutine and messages will be passed
	in to the decoder over a channel, freeing the input to start processing
	the next chunk of incoming or available data. If true, then any decoding
	will happen synchronously and message delivery will not return control to
	the input until after decoding has completed. Defaults to false.
- send_decode_failures (bool, optional):
	If false, then if an attempt to decode a message fails then Heka will log
	an error message and then drop the message. If true, then in addition to
	logging an error message, decode failure will cause the original,
	undecoded message to be tagged with a `decode_failure` field (set to true)
	and delivered to the router for possible further processing.
- splitter (string, optional)
	Splitter to be used by the input. This should refer to the name of a
	registered splitter plugin configuration. It specifies how the input
	should split the incoming data stream into individual records prior to
	decoding and/or injection to the router. Typically defaults to
	"NullSplitter", although certain inputs override this with a different
	default value.

.. _config_amqp_input:
.. include:: /config/inputs/amqp.rst

.. _config_docker_log_input:
.. include:: /config/inputs/docker_log.rst

.. _config_file_polling_input:
.. include:: /config/inputs/file_polling.rst

.. _config_http_input:
.. include:: /config/inputs/http.rst

.. _config_http_listen_input:
.. include:: /config/inputs/httplisten.rst

.. _config_kafka_input:
.. include:: /config/inputs/kafka.rst

.. _config_logstreamer_input:
.. include:: /config/inputs/logstreamer.rst

.. _config_process_input:
.. include:: /config/inputs/process.rst

.. _config_process_directory_input:
.. include:: /config/inputs/processdir.rst

.. _config_stat_accum_input:
.. include:: /config/inputs/stataccum.rst

.. _config_statsd_input:
.. include:: /config/inputs/statsd.rst

.. _config_tcp_input:
.. include:: /config/inputs/tcp.rst

.. _config_udp_input:
.. include:: /config/inputs/udp.rst

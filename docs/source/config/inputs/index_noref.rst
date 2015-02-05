
======
Inputs
======

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

.. include:: /config/inputs/amqp.rst
   :start-line: 1

.. include:: /config/inputs/docker_log.rst
   :start-line: 1

.. include:: /config/inputs/file_polling.rst
   :start-line: 1

.. include:: /config/inputs/http.rst
   :start-line: 1

.. include:: /config/inputs/httplisten.rst
   :start-line: 1

.. include:: /config/inputs/kafka.rst
   :start-line: 1

.. include:: /config/inputs/logstreamer.rst
   :start-line: 1

.. include:: /config/inputs/process.rst
   :start-line: 1

.. include:: /config/inputs/processdir.rst
   :start-line: 1

.. include:: /config/inputs/stataccum.rst
   :start-line: 1

.. include:: /config/inputs/statsd.rst
   :start-line: 1

.. include:: /config/inputs/tcp.rst
   :start-line: 1

.. include:: /config/inputs/udp.rst
   :start-line: 1


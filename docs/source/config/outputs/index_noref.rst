
=======
Outputs
=======

Common Output Parameters
========================

There are some configuration options that are universally available to all
Heka output plugins. These will be consumed by Heka itself when Heka
initializes the plugin and do not need to be handled by the plugin-specific
initialization code.

- message_matcher (string, optional):
    Boolean expression, when evaluated to true passes the message to the filter
    for processing. Defaults to matching nothing. See: :ref:`message_matcher`
- message_signer (string, optional):
    The name of the message signer.  If  specified only messages with this
    signer  are passed to the filter for processing.
- ticker_interval (uint, optional):
    Frequency (in seconds) that a timer event will be sent to the filter.
    Defaults to not sending timer events.

.. include:: /config/outputs/amqp.rst

.. include:: /config/outputs/carbon.rst

.. include:: /config/outputs/dashboard.rst

.. include:: /config/outputs/elasticsearch.rst

.. include:: /config/outputs/file.rst

.. include:: /config/outputs/log.rst

.. include:: /config/outputs/nagios.rst

.. include:: /config/outputs/smtp.rst

.. include:: /config/outputs/tcp.rst

.. include:: /config/outputs/whisper.rst

.. _config_filters:

=======
Filters
=======

.. _config_common_filter_parameters:

Common Filter Parameters
========================

There are some configuration options that are universally available to all
Heka filter plugins. These will be consumed by Heka itself when Heka
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

.. _config_counter_filter:
.. include:: /config/filters/counter.rst

.. _config_stat_filter:
.. include:: /config/filters/stat.rst

.. _config_sandbox_filter:
.. include:: /config/filters/sandbox.rst

.. _config_sandbox_manager_filter:
.. include:: /config/filters/sandboxmanager.rst


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
- can_exit (bool, optional)
    .. versionadded:: 0.7
    
    Whether or not this plugin can exit without causing Heka to shutdown.
    Defaults to false for non-sandbox filters, and true for sandbox filters.

Available Filter Plugins
========================

.. toctree::
   :maxdepth: 1

   cbuf_delta
   cbuf_delta_by_host
   counter
   cpu_stats
   disk_stats
   frequent_items
   heka_memstat
   http_status
   load_avg
   mem_stats
   message_failures
   message_schema
   mysql_slow_query
   sandbox
   sandboxmanager
   stat
   stats_graph
   unique_items

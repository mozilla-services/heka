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

.. _config_circular_buffer_delta_agg_filter:

Circular Buffer Delta Aggregator
================================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/cbufd_aggregator.lua
   :start-after: --[[
   :end-before: --]]

.. _config_circular_buffer_delta_agg_by_host:

CBuf Delta Aggregator By Hostname
=================================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/cbufd_host_aggregator.lua
   :start-after: --[[
   :end-before: --]]

.. _config_counter_filter:

.. include:: /config/filters/counter.rst

.. _config_frequent_items_filter:

Frequent Items
==============

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/frequent_items.lua
   :start-after: --[[
   :end-before: --]]

.. _config_memstat_filter:

Heka Memory Statistics
======================

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/heka_memstat.lua
   :start-after: --[[
   :end-before: --]]

.. _config_message_schema_filter:

Heka Message Schema
===================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/heka_message_schema.lua
   :start-after: --[[
   :end-before: --]]

.. _config_http_status_graph_filter:

HTTP Status Graph
=================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/http_status.lua
   :start-after: --[[
   :end-before: --]]

.. _config_mysql_slow_query_filter:

MySQL Slow Query
================

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/mysql_slow_query.lua
   :start-after: --[[
   :end-before: --]]

.. _config_stat_filter:
.. include:: /config/filters/stat.rst

.. _config_sandbox_filter:
.. include:: /config/filters/sandbox.rst

.. _config_sandbox_manager_filter:
.. include:: /config/filters/sandboxmanager.rst

.. _config_unique_items_filter:

Unique Items
============

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/unique_items.lua
   :start-after: --[[
   :end-before: --]]


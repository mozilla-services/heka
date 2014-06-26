
=======
Filters
=======

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

Circular Buffer Delta Aggregator
================================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/cbufd_aggregator.lua
   :start-after: --[[
   :end-before: --]]

CBuf Delta Aggregator By Hostname
=================================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/cbufd_host_aggregator.lua
   :start-after: --[[
   :end-before: --]]

.. include:: /config/filters/counter.rst

Frequent Items
==============

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/frequent_items.lua
   :start-after: --[[
   :end-before: --]]

Heka Memory Statistics
======================

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/heka_memstat.lua
   :start-after: --[[
   :end-before: --]]

Heka Message Schema
===================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/heka_message_schema.lua
   :start-after: --[[
   :end-before: --]]

HTTP Status Graph
=================

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/filters/http_status.lua
   :start-after: --[[
   :end-before: --]]

MySQL Slow Query
================

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/mysql_slow_query.lua
   :start-after: --[[
   :end-before: --]]

.. include:: /config/filters/stat.rst

.. include:: /config/filters/sandbox.rst

.. include:: /config/filters/sandboxmanager.rst

Unique Items
============

.. versionadded:: 0.6

.. include:: /../../sandbox/lua/filters/unique_items.lua
   :start-after: --[[
   :end-before: --]]


.. _sandboxfilter:

.. include:: ../config/filters/sandbox.rst
   :start-line: 1

.. _sandboxfilters:

Available Sandbox Filters
-------------------------

Circular Buffer Delta Aggregator
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/cbufd_aggregator.lua
   :start-after: --[[
   :end-before: --]]

Circular Buffer Delta Aggregator (by hostname)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/cbufd_host_aggregator.lua
   :start-after: --[[
   :end-before: --]]

CPU Stats Filter
^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/filters/procstat.lua
   :start-after: --[[
   :end-before: --]]

Disk Stats Filter
^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/filters/diskstats.lua
   :start-after: --[[
   :end-before: --]]

Frequent Items
^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/frequent_items.lua
   :start-after: --[[
   :end-before: --]]

Heka Memory Statistics (self monitoring)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/heka_memstat.lua
   :start-after: --[[
   :end-before: --]]

Heka Message Schema (Message Documentation)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/heka_message_schema.lua
   :start-after: --[[
   :end-before: --]]

Heka Process Message Failures (self monitoring)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/heka_process_message_failures.lua
   :start-after: --[[
   :end-before: --]]

HTTP Status Graph
^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/http_status.lua
   :start-after: --[[
   :end-before: --]]

Load Average Filter
^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/filters/loadavg.lua
   :start-after: --[[
   :end-before: --]]

Memory Stats Filter
^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/filters/memstats.lua
   :start-after: --[[
   :end-before: --]]

MySQL Slow Query
^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/filters/mysql_slow_query.lua
   :start-after: --[[
   :end-before: --]]

Stats Graph
^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/stat_graph.lua
   :start-after: --[[
   :end-before: --]]

Unique Items
^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/filters/unique_items.lua
   :start-after: --[[
   :end-before: --]]


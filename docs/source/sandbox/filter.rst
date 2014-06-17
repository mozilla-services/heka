.. _sandboxfilter:

.. include:: ../config/filters/sandbox.rst

.. _sandboxfilters:

Available Sandbox Filters
=========================

Circular Buffer Delta Aggregator
--------------------------------
.. include:: ../../../sandbox/lua/filters/cbufd_aggregator.lua
   :start-after: --[[
   :end-before: --]]

Circular Buffer Delta Aggregator (by hostname)
----------------------------------------------
.. include:: ../../../sandbox/lua/filters/cbufd_host_aggregator.lua
   :start-after: --[[
   :end-before: --]]

Frequent Items
--------------
.. include:: ../../../sandbox/lua/filters/frequent_items.lua
   :start-after: --[[
   :end-before: --]]

Heka Memory Statistics (self monitoring)
----------------------------------------
.. include:: ../../../sandbox/lua/filters/heka_memstat.lua
   :start-after: --[[
   :end-before: --]]

Heka Message Schema (Message Documentation)
-------------------------------------------
.. include:: ../../../sandbox/lua/filters/heka_message_schema.lua
   :start-after: --[[
   :end-before: --]]

HTTP Status Graph
-----------------
.. include:: ../../../sandbox/lua/filters/http_status.lua
   :start-after: --[[
   :end-before: --]]

Unique Items
------------
.. include:: ../../../sandbox/lua/filters/unique_items.lua
   :start-after: --[[
   :end-before: --]]


.. _sandboxfilter:

.. include:: ../config/decoders/sandbox.rst

.. _sandboxfilters:

Available Sandbox Filters
=========================

Circular Buffer Delta Aggregator
--------------------------------
.. literalinclude:: ../../../sandbox/lua/filters/cbufd_aggregator.lua
   :language: lua 
   :lines: 5-26

Circular Buffer Delta Aggregator (by hostname)
----------------------------------------------
.. literalinclude:: ../../../sandbox/lua/filters/cbufd_host_aggregator.lua
   :language: lua 
   :lines: 5-35

Frequent Items
--------------
.. literalinclude:: ../../../sandbox/lua/filters/frequent_items.lua
   :language: lua 
   :lines: 5-31

Heka Message Schema (Message Documentation)
-------------------------------------------
.. literalinclude:: ../../../sandbox/lua/filters/heka_message_schema.lua
   :language: lua 
   :lines: 5-28

HTTP Status Graph
-----------------
.. literalinclude:: ../../../sandbox/lua/filters/http_status.lua
   :language: lua 
   :lines: 5-25

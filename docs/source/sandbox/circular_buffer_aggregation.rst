.. _circular_buffer_aggregator:

Circular Buffer Aggregator
==========================

Aggregrates the circular buffer delta data produced by other Heka systems to 
create a higher level aggregate view. i.e., host -> data center -> world 

.. literalinclude:: ../../../sandbox/lua/testsupport/cbufd_aggregator.lua
    :language: lua 

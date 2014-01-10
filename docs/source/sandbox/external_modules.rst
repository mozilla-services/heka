.. _external_modules:

Using External Grammar Modules to Parse and Transform
=====================================================
This example:

1) Decodes a JSON payload.
2) Parses a RFC3339 compliant date/time stamp, using an external grammar module,
   and converts in to nanoseconds since the Unix epoch.
3) Parses a RFC5424 severity keyword to the corresponding code.
4) Creates field elements from JSON elements and applies representation metadata.

.. literalinclude:: ../../../sandbox/lua/testsupport/telemetry_server.lua
    :language: lua 


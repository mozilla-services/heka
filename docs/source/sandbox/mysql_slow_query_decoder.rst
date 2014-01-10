.. _mysql_slow_query_decoder:

MySQL Slow Query Log Decoder
============================
This plugin is an example of how to decode a multi-line log entry directly into
a Heka message using the LPEG parser.

.. literalinclude:: ../../../sandbox/lua/testsupport/mysql_slow_query_decoder.lua
    :language: lua 

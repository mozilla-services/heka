.. _json_payload_transform:

JSON Payload Transform
======================
Alters a date/time string in the JSON payload to be RFC3339 compliant. 
In this example the JSON is parsed, transformed and re-injected into the payload.

.. literalinclude:: ../../../sandbox/lua/testsupport/json_payload_transform.lua
    :language: lua 

Alters a date/time string in the JSON payload to be RFC3339 compliant. 
In this example a search and replace is performed on the JSON text and 
re-injected into the payload.  

.. literalinclude:: ../../../sandbox/lua/testsupport/json_payload_transform_sr.lua
    :language: lua 

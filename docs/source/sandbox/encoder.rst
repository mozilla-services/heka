.. _sandboxencoder:

.. include:: ../config/encoders/sandbox.rst
   :start-line: 1

.. _sandboxencoders:

Available Sandbox Encoders
--------------------------

Alert Encoder
^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/alert.lua
   :start-after: --[[
   :end-before: --]]

CBUF Librato Encoder
^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/cbuf_librato.lua
   :start-after: --[[
   :end-before: --]]

ESPayloadEncoder
^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/es_payload.lua
   :start-after: --[[
   :end-before: --]]

Schema InfluxDB Encoder
^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/schema_influx.lua
   :start-after: --[=[
   :end-before: --]=]

Schema InfluxDB Write Encoder
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/schema_influx_write.lua
   :start-after: --[=[
   :end-before: --]=]

Statmetric Influx Encoder
^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/encoders/statmetric_influx.lua
   :start-after: --[=[
   :end-before: --]=]

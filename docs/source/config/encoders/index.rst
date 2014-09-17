.. versionadded:: 0.6

.. _config_encoders:

========
Encoders
========

.. _config_alert_encoder:

Alert Encoder
=============

.. include:: /../../sandbox/lua/encoders/alert.lua
   :start-after: --[[
   :end-before: --]]

.. _config_esjsonencoder:
.. include:: /config/encoders/esjson.rst

.. _config_eslogstashv0encoder:
.. include:: /config/encoders/eslogstashv0.rst

.. _config_espayload:

ESPayloadEncoder
================

.. include:: /../../sandbox/lua/encoders/es_payload.lua
   :start-after: --[[
   :end-before: --]]

.. _config_payloadencoder:
.. include:: /config/encoders/payload.rst

.. _config_protobufencoder:
.. include:: /config/encoders/protobuf.rst

.. _config_rstencoder:
.. include:: /config/encoders/rst.rst

.. _config_sandboxencoder:
.. include:: /config/encoders/sandbox.rst

.. _config_schema_influx_encoder:

Schema InfluxDB Encoder
=======================

.. include:: /../../sandbox/lua/encoders/schema_influx.lua
	:start-after: --[=[
	:end-before: --]=]

.. _config_statmetric_influx:

StatMetric InfluxDB Encoder
===========================

.. include:: /../../sandbox/lua/encoders/statmetric_influx.lua
	:start-after: --[=[
	:end-before: --]=]

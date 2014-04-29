.. _config_decoders:

========
Decoders
========

Apache Access Log Decoder
=========================

.. versionadded:: 0.6
.. include:: /../../sandbox/lua/decoders/apache_access.lua
   :start-after: --[[
   :end-before: --]]

.. _config_multidecoder:
.. include:: /config/decoders/multi.rst

.. _config_nginx_access_log_decoder:

Nginx Access Log Decoder
========================

.. versionadded:: 0.5
.. include:: /../../sandbox/lua/decoders/nginx_access.lua
   :start-after: --[[
   :end-before: --]]

Nginx Error Log Decoder
=======================

.. versionadded:: 0.6
.. include:: /../../sandbox/lua/decoders/nginx_error.lua
   :start-after: --[[
   :end-before: --]]

.. _config_payloadjson_decoder:
.. include:: /config/decoders/payload_json.rst

.. _config_payloadregex_decoder:
.. include:: /config/decoders/payload_regex.rst

.. _config_payload_xml_decoder:
.. include:: /config/decoders/payload_xml.rst

.. _config_protobuf_decoder:
.. include:: /config/decoders/protobuf.rst

.. _config_rsyslog_decoder:

Rsyslog Decoder
===============

.. versionadded:: 0.5

.. include:: /../../sandbox/lua/decoders/rsyslog.lua
   :start-after: --[[
   :end-before: --]]

.. _config_sandboxdecoder:
.. include:: /config/decoders/sandbox.rst

.. _config_scribbledecoder:
.. include:: /config/decoders/scribble.rst

.. _config_statstofieldsdecoder:
.. include:: /config/decoders/stats_to_fields.rst

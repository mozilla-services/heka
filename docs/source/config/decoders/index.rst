.. _config_decoders:

========
Decoders
========

.. _config_apache_access_log_decoder:

Apache Access Log Decoder
=========================

.. versionadded:: 0.6
.. include:: /../../sandbox/lua/decoders/apache_access.lua
   :start-after: --[[
   :end-before: --]]

.. _config_graylog_extended_log_format_decoder:

Graylog Extended Log Format Decoder
===================================

.. versionadded:: 0.8
.. include:: /../../sandbox/lua/decoders/graylog_extended.lua
   :start-after: --[[
   :end-before: --]]

.. _config_geoip_decoder:
.. include:: /config/decoders/geoip_decoder.rst

.. _config_multidecoder:
.. include:: /config/decoders/multi.rst

.. _config_linux_disk_stats_decoder:

Linux Disk Stats Decoder
========================

.. versionadded:: 0.7
.. include:: /../../sandbox/lua/decoders/linux_diskstats.lua
   :start-after: --[[
   :end-before: --]]

.. _config_linux_load_avg_decoder:

Linux Load Average Decoder
==========================

.. versionadded:: 0.7
.. include:: /../../sandbox/lua/decoders/linux_loadavg.lua
   :start-after: --[[
   :end-before: --]]

.. _config_linux_mem_stats_decoder:

Linux Memory Stats Decoder
==========================

.. versionadded:: 0.7
.. include:: /../../sandbox/lua/decoders/linux_memstats.lua
   :start-after: --[[
   :end-before: --]]

.. _config_mysql_slow_query_log_decoder:

MySQL Slow Query Log Decoder
============================

.. versionadded:: 0.6
.. include:: /../../sandbox/lua/decoders/mysql_slow_query.lua
   :start-after: --[[
   :end-before: --]]

.. _config_nginx_access_log_decoder:

Nginx Access Log Decoder
========================

.. versionadded:: 0.5
.. include:: /../../sandbox/lua/decoders/nginx_access.lua
   :start-after: --[[
   :end-before: --]]

.. _config_nginx_error_log_decoder:

Nginx Error Log Decoder
=======================

.. versionadded:: 0.6
.. include:: /../../sandbox/lua/decoders/nginx_error.lua
   :start-after: --[[
   :end-before: --]]

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

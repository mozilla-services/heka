.. _sandboxdecoder:

.. include:: ../config/decoders/sandbox.rst
   :start-line: 1

.. _sandboxdecoders:

Available Sandbox Decoders
--------------------------

Apache Access Log Decoder
^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/apache_access.lua
   :start-after: --[[
   :end-before: --]]

Graylog Extended Log Format Decoder
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/graylog_extended.lua
   :start-after: --[[
   :end-before: --]]

Linux CPU Stats Decoder
^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/decoders/linux_procstat.lua
   :start-after: --[[
   :end-before: --]]

Linux Disk Stats Decoder
^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/decoders/linux_diskstats.lua
   :start-after: --[[
   :end-before: --]]

Linux Load Average Decoder
^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/decoders/linux_loadavg.lua
   :start-after: --[[
   :end-before: --]]

Linux Memory Stats Decoder
^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: /../../sandbox/lua/decoders/linux_memstats.lua
   :start-after: --[[
   :end-before: --]]

MySQL Slow Query Log Decoder
^^^^^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/mysql_slow_query.lua
   :start-after: --[[
   :end-before: --]]

Nginx Access Log Decoder
^^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/nginx_access.lua
   :start-after: --[[
   :end-before: --]]

Nginx Error Log Decoder
^^^^^^^^^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/nginx_error.lua
   :start-after: --[[
   :end-before: --]]

Rsyslog Decoder
^^^^^^^^^^^^^^^
.. include:: ../../../sandbox/lua/decoders/rsyslog.lua
   :start-after: --[[
   :end-before: --]]

Externally Available Sandbox Decoders
-------------------------------------

- `GZip Payload Decoder <https://github.com/mozilla-services/data-pipeline/blob/master/heka/sandbox/decoders/decompress_payload.lua>`_

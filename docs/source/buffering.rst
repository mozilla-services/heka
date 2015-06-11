.. _buffering:

=====================
Configuring Buffering
=====================

All filter and output plugins support the use of a disk based message queue.
If ``use_buffering`` is set to true, then the router will deliver messages that
match the plugin's :ref:`message matcher <message_matcher>` to the queue
buffer, and the plugin will read from the queue to get messages to process,
instead of the handoff happening in the process RAM via Go channels. This
improves message delivery reliability and allows plugins to reprocess messages
from the queue in cases where upstream servers are down or Heka is recovering
from a hard shutdown.

Each queue buffer supports a few configuration settings in addition to any
options which the plugin might support. These can be specified in a sub-section
of the plugin's TOML configuration section entitled ``buffering``.

Buffering configuration settings
================================

- max_file_size (uint64)
  The maximum size (in bytes) of a single file in the queue buffer. When a
  message would increase a queue file to greater than this size, the message
  will be written into a new file instead. Defaults to 128MiB. Value cannot
  be zero, if zero is specified the default will instead be used.

- max_buffer_size (uint64)
  Maximum amount of disk space (in bytes) that the entire queue buffer can
  consume. Defaults to 0, or no limit. The action taken when the maximum buffer
  size is reached is determined by the ``full_action`` setting.

- full_action (string)
  The action Heka will take if the queue buffer grows to larger than the
  maximum specified by the ``max_buffer_size`` setting. Must be one of the
  following values. Defaults to ``shutdown``, although specific plugins might
  override this default with a default of their own:

  * ``shutdown``: Heka will stop all processing and attempt a clean shutdown.

  * ``drop``: Heka will drop the current message and will continue to process
              future messages.

  * ``block``: Heka will pause message delivery, applying back pressure through
               the router to the inputs. Delivery will resume if and when the
               queue buffer size reduces to below the specified maximum.

- cursor_update_count (uint)
  A plugin is responsible for notifying the queue buffer when a message has
  been processed by calling an ``UpdateCursor`` method on the
  PluginRunner. Some plugins call this for every message, while others call it
  only periodically after processing a large batch of messages. This setting
  specifies how many ``UpdateCursor`` calls must be made before the cursor
  location is flushed to disk. Defaults to 1, although specific plugins might
  override this default with a default of their own. Value cannot be zero, if
  zero is specified the default will be used instead.

Buffering Default Values
========================

Please note that if you provide a `buffering` subsection for your plugin
configuration, it is best to specify *all* of the available settings. In cases
where the plugin specifies a non-standard default for one or more of these
values, that default will only be applied if you omit the `buffering`
subsection altogether. If you specify any of the values, it is expected that
you will specify all of the values.

Sample Buffering Configuration
==============================

The following is a sample TcpOutput configuration showing the use of buffering.

.. code-block:: ini

    [TcpOutput]
    message_matcher = "Type !~ /^heka/"
    address = "upstream.example.com:5565"
    keep_alive = true
    use_buffering = true

        [TcpOutput.buffering]
        max_file_size = 268435456  # 256MiB
        max_buffer_size = 1073741824  # 1GiB
        full_action = "block"
        cursor_update_count = 100

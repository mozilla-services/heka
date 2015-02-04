
.. versionadded:: 0.9

=========
Splitters
=========

Common Splitter Parameters
==========================

There are some configuration options that are universally available to all
Heka splitter plugins. These will be consumed by Heka itself when Heka
initializes the plugin and do not need to be handled by the plugin-specific
initialization code.

- keep_truncated (bool, optional):
	If true, then any records that exceed the capacity of the input buffer
	will still be delivered in their truncated form. If false, then these
	records will be dropped. Defaults to false.
- use_message_bytes (bool, optional):
	Most decoders expect to find the raw, undecoded input data stored as the
	payload of the received Heka Message struct. Some decoders, however, such
	as the ProtobufDecoder, expect to receive a blob of bytes representing an
	entire Message struct, not just the payload. In this case, the data is
	expected to be found on the MsgBytes attribute of the Message's
	PipelinePack. If `use_message_bytes` is true, then the data will be
	written as a byte slice to the MsgBytes attribute, otherwise it will be
	written as a string to the Message payload. Defaults to false in most
	cases, but defaults to true for the HekaFramingSplitter, which is almost
	always used with the ProtobufDecoder.

.. include:: /config/splitters/heka_framing.rst

.. include:: /config/splitters/null.rst

.. include:: /config/splitters/regex.rst

.. include:: /config/splitters/token.rst
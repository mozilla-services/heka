. _config_heka_framing_splitter

Heka Framing Splitter
=====================

Plugin Name: **HekaFramingSplitter**

A HekaFramingSplitter is used to split streams of data that use Heka's built-
in :ref:`stream_framing`, with a protocol buffers encoded message header
supporting HMAC key authentication.

A default configuration of the HekaFramingSplitter is automatically registered
as an available splitter plugin as "HekaFramingSplitter", so it is only
necessary to add an additional TOML section if you want to use an instance of
the splitter with settings other than the default.

Config:

- signer:
	Optional TOML subsection. Section name consists of a signer name, underscore,
	and numeric version of the key.

	- hmac_key (string):
	    The hash key used to sign the message.

- use_message_bytes (bool, optional):
	The HekaFramingSplitter is almost always used in concert with an instance
	of ProtobufDecoder, which expects the protocol buffer message data to be
	available in the PipelinePack's MsgBytes attribute, so `use_message_bytes`
	defaults to true.

- skip_authentication (bool, optional):
	Usually if a HekaFramingSplitter identifies an incorrectly signed message,
	that message will be silently dropped. In some cases, however, such as
	when loading a stream of protobuf encoded Heka messages from a file system
	file, it may be desirable to skip authentication altogether. Setting this
	to true will do so. Defaults to false.

Example:

.. code-block:: ini

	[acl_splitter]
	type = "HekaFramingSplitter"

	  [acl_splitter.signer.ops_0]
	  hmac_key = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
	  [acl_splitter.signer.ops_1]
	  hmac_key = "xdd908lfcgikauexdi8elogusridaxoalf"

	  [acl_splitter.signer.dev_1]
	  hmac_key = "haeoufyaiofeugdsnzaogpi.ua,dp.804u"

	[tcp_control]
	type = "TcpInput"
	address = ":5566"
	splitter = "acl_splitter"

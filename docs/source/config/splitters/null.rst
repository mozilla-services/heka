.. _config_null_splitter:

Null Splitter
=============

Plugin Name: **NullSplitter**

The NullSplitter is used in cases where the incoming data is already naturally
divided into logical messages, such that Heka doesn't need to do any further
splitting. For instance, when used in conjunction with a UdpInput, the
contents of each UDP packet will be made into a separate message.

Note that this means generally the NullSplitter should not be used with a
stream oriented input transport, such as with TcpInput or LogstreamerInput. If
this is done then the splitting will be arbitrary, each message will contain
whatever happens to be the contents of a particular read operation.

The NullSplitter has no configuration options, and is automatically registered
as an available splitter plugin of the name "NullSplitter", so it doesn't
require a separate TOML configuration section.

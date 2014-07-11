
.. versionadded:: 0.7

UdpOutput
=========

Output plugin that delivers Heka message data to a specified UDP or Unix
datagram socket location.

Config:

- net (string, optional):
	Network type to use for communication. Must be one of "udp", "udp4",
	"udp6", or "unixgram". "unixgram" option only available on systems that
	support Unix datagram sockets. Defaults to "udp".
- address (string):
	Address to which we will be sending the data. Must be IP:port for net
	types of "udp", "udp4", or "udp6". Must be a path to a Unix datagram
	socket file for net type "unixgram".
- local_address (string, optional):
	Local address to use on the datagram packets being generated. Must be
	IP:port for net types of "udp", "udp4", or "udp6". Must be a path to a
	Unix datagram socket file for net type "unixgram".
- encoder (string):
	Name of registered encoder plugin that will extract and/or serialized data
	from the Heka message.

Example:

.. code-block:: ini

	[PayloadEncoder]

	[UdpOutput]
	address = "myserver.example.com:34567"
	encoder = "PayloadEncoder"
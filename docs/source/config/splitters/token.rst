.. _config_token_splitter:

Token Splitter
==============

Plugin Name: **TokenSplitter**

A TokenSplitter is used to split an incoming data stream on every occurrence
of a single, one byte token character. The token will be included as the final
character in the returned record.

A default configuration of the TokenSplitter is automatically registered as an
available splitter plugin as "TokenSplitter", so additional TOML sections
don't need to be added unless you want to specify a different delimiter.

Config:

- delimiter (string, optional):
	String representation of the byte token to be used as message delimiter.
	Defaults to "\n".

Example:

.. code-block:: ini

	[split_on_space]
	type = "TokenSplitter"
	delimiter = " "

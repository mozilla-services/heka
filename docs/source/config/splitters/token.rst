.. _config_token_splitter:

Token Splitter
==============

Plugin Name: **TokenSplitter**

A TokenSplitter is used to split an incoming data stream on every occurrence
(or every Nth occurrence) of a single, one byte token character. The token
will be included as the final character in the returned record.

A default configuration of the TokenSplitter (i.e. splitting on every newline)
is automatically registered as an available splitter plugin as
"TokenSplitter", so additional TOML sections don't need to be added unless you
want to use different settings.

Config:

- delimiter (string, optional):
	String representation of the byte token to be used as message delimiter.
	Defaults to `"\\n"`.

- count (uint, optional):
	Number of instances of the delimiter that should be encountered before
	returning a record. Defaults to 1. Setting to 0 has no effect, 0 and 1
	will be treated identically. Often used in conjunction with the
	:ref:`deliver_incomplete_final <config_common_splitter_parameters>`
	option set to true, to ensure trailing partial records are still
	delivered.

Example:

.. code-block:: ini

	[split_on_space]
	type = "TokenSplitter"
	delimiter = " "

	[split_every_50th_newline_keep_partial]
	type = "TokenSplitter"
	count = 50
	deliver_incomplete_final = true

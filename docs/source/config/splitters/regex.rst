.. _config_regex_splitter:

Regex Splitter
==============

Plugin Name: **RegexSplitter**

A RegexSplitter considers any text that matches a specified regular expression
to represent a boundary on which records should be split. The regular
expression may consist of exactly one capture group. If a capture group is
specified, then the captured text will be included in the returned record. If
not, then the returned record will not include the text that caused the
regular expression match.

Config:

- delimiter (string)
	Regular expression to be used as the record boundary. May contain zero or
	one specified capture groups.
- delimiter_eol (bool, optional):
	Specifies whether the contents of a delimiter capture group should be
	appended to the end of a record (true) or prepended to the beginning
	(false). Defaults to true. If the delimiter expression does not specify a
	capture group, this will have no effect.

Example:

.. code-block:: ini

	[mysql_slow_query_splitter]
	type = "RegexSplitter"
	delimiter = '\n(# User@Host:)'
	delimiter_eol = false

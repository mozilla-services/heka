.. _config_pattern_grouping_splitter:

.. versionadded:: 0.11

PatternGrouping Splitter
========================

Plugin Name: **PatternGroupingSplitter**

A PatternGroupingSplitter is an extension of the RegexSplitter for use cases
where a single log record may sometimes span multiple delimiter patterns. The
delimiter pattern is first used to break the stream into records as it is in
the RegexSplitter. But then a second pass is taken over the split records and
the grouping pattern is used to rejoin matching, contiguous records into a
single record.

This splitter is useful for things like Stacktraces intermixed with other logs
in the same stream. The normal record delimiter pattern might be a line feed
and so the first pass by the splitter splits the stream into lines. In the
second pass, the grouping pattern is used to identify the lines of a stacktrace
that should be grouped back together to keep the trace in a single log record.
In this way you can get a single record for each stacktrace, but all other
lines are split and recorded separately.

Another example use case is where an application dumps configuration values to
the log on a timed basis and these values should all be grouped into a single
record, rather than one line at a time.

The performance characteristics of the PatternGroupingSplitter are noticeably
worse than a RegexSplitter. The RegexSplitter is optimized to break one record
off at a time, matching only the first occurrence of the record delimiter
before processing the record. The PatternGroupingSplitter must break apart all
of the records in a single read from the stream, then re-test each line against
the grouping pattern. This can result in the same line being processed by the
delimiter pattern many times. Careful consideration should be made as to
whether or not this performance tradeoff is tolerable.

Finally, the PatternGroupingSplitter, unlike the RegexSplitter, will always
include the delimiter pattern for each line and also at the end of the record.

Config:

- delimiter (string)
	Regular expression to be used as the record boundary. May contain zero or
	one specified capture groups.
- grouping (string)
	Regular expression to be used to regroup matching records into a single
	final record. Any contiguous lines matching this expression will become
	a single record.
- max_lines (int, optional)
    The maximum number of records to process in a single splitting operation
	with the delimiter pattern. This is used to tune for performance by
	helping to limit the number of times a single line will be re-parsed
	with the delimiter expression. The knock-on effect is that this will also
	set the upper bound on the number of lines that can be grouped into a
	single record. Defaults to 99.

Example:

.. code-block:: ini

	[stacktrace_grouping_splitter]
	type = "PatternGroupingSplitter"
	delimiter = '\n'
	grouping = '(\] FATAL )|(\A\s*.+Exception: .)|(at \S+\(\S+\))|(\A\s+... \d+ more)|(\A\s*Caused by:.)|(\A\s*Grave:)'

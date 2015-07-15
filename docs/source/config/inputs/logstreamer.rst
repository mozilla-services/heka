.. _config_logstreamer_input:

Logstreamer Input
==================

.. versionadded:: 0.5

Plugin Name:: **LogstreamerInput**

Tails a single log file, a sequential single log source, or multiple log sources
of either a single logstream or multiple logstreams.

.. seealso:: :ref:`Complete documentation with examples <logstreamerplugin>`

Config:

- hostname (string):
    The hostname to use for the messages, by default this will be the
    machine's qualified hostname. This can be set explicitly to ensure
    it's the correct name in the event the machine has multiple
    interfaces/hostnames.
- oldest_duration (string):
    A time duration string (e.x. "2s", "2m", "2h"). Logfiles with a
    last modified time older than ``oldest_duration`` ago will not be included
    for parsing. Defaults to "720h" (720 hours, i.e. 30 days).
- journal_directory (string):
    The directory to store the journal files in for tracking the location that
    has been read to thus far. By default this is stored under heka's base
    directory.
- log_directory (string):
    The root directory to scan files from. This scan is recursive so it
    should be suitably restricted to the most specific directory this
    selection of logfiles will be matched under. The log_directory path will
    be prepended to the file_match.
- rescan_interval (int):
    During logfile rotation, or if the logfile is not originally
    present on the system, this interval is how often the existence of
    the logfile will be checked for. The default of 5 seconds is
    usually fine. This interval is in milliseconds.
- file_match (string):
    Regular expression used to match files located under the
    ``log_directory``. This regular expression has ``$`` added to the end
    automatically if not already present, and ``log_directory`` as the prefix.
    WARNING: file_match should typically be delimited with single quotes,
    indicating use of a raw string, rather than double quotes, which require
    all backslashes to be escaped. For example, `'access\\.log'` will work as
    expected, but `"access\\.log"` will not, you would need `"access\\\\.log"`
    to achieve the same result.
- priority (list of strings):
    When using sequential logstreams, the priority is how to sort the logfiles
    in order from oldest to newest.
- differentiator (list of strings):
    When using multiple logstreams, the differentiator is a set of strings that
    will be used in the naming of the logger, and portions that match a captured
    group from the ``file_match`` will have their matched value substituted in.
- translation (hash map of hash maps of ints):
    A set of translation mappings for matched groupings to the ints to use for
    sorting purposes.
- splitter (string, optional):
    Defaults to "TokenSplitter", which will split the log stream into one
    Heka message per line.

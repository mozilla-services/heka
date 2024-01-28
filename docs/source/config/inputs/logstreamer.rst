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
- glob_pattern (string, optional):
    By default, the method for scanning the filesystem uses the filepath.Walk()
    method. That will traverse every directory/file down "log_dir" and check
    to see if it matches "file_match". This can be slower depending on the
    complexity of the filesystem. "glob_pattern" provides an optional
    alternative method for scanning the filesystem by quickly ruling out
    locations that would otherwise be scanned. This is still used in conjunction
    with "log_dir" and "file_match. Uses `format
    <https://golang.org/pkg/path/filepath/#Match>`_. (e.x. `"/var/log/*/*.log*"`).
- priority (list of strings):
    When using sequential logstreams, the priority is how to sort the logfiles
    in order from oldest to newest.
- differentiator (list of strings):
    When using multiple logstreams, the differentiator is a set of strings that
    will be used in the naming of the logger, and portions that match a captured
    group from the ``file_match`` will have their matched value substituted in.
    Only the last (according to priority) file per differentiator is kept opened.
- translation (hash map of hash maps of ints):
    A set of translation mappings for matched groupings to the ints to use for
    sorting purposes.
- splitter (string, optional):
    Defaults to "TokenSplitter", which will split the log stream into one
    Heka message per line.

.. versionadded:: 0.10

- check_data_interval (string)
    A time duration string. This interval is how often streams will be checked
    for new data. Defaults to "250ms". If the plugin processes many logstreams,
    you may increase this value to reduce the CPU load.

.. versionadded:: 0.11

- initial_tail (bool, optional, default: false):
    If this setting is true, when there is no cursor file for a given stream
    (which is always the case when reading a stream for the first time) then
    the input will start from the end of the stream instead of the
    beginning. If a cursor file exists, the input will attempt to continue from
    the specified cursor location, as always.

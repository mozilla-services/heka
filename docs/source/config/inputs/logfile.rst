
LogfileInput
============

.. deprecated:: 0.5
    This input has been superseded by :ref:`config_logstreamer_input`.

Tails a single log file, creating a message for each line in the file being
monitored. Files are read in their entirety, and watched for changes. This
input gracefully handles log rotation via the file moving but may lose a few
log lines if using the "truncation" method of log rotation. It's recommended
to use log rotation schemes that move the file to another location to avoid
possible loss of log lines.

In the event the log file does not currently exist, it will be placed in an
internal discover list, and checked for existence every `discover_interval`
milliseconds (5000ms or 5s by default).

A single LogfileInput can only be used to read a single file. If you have
multiple identical files spread across multiple directories (e.g. a
`/var/log/hosts/<HOSTNAME>/app.log` structure, where each <HOSTNAME> folder
contains a log file originating from a separate host), you'll want to use the
:ref:`config_logfile_directory_manager_input`.

Parameters:

- logfile (string):
    Each LogfileInput can have a single logfile to monitor.
- hostname (string):
    The hostname to use for the messages, by default this will be the
    machines qualified hostname. This can be set explicitly to ensure
    its the correct name in the event the machine has multiple
    interfaces/hostnames.
- discover_interval (int):
    During logfile rotation, or if the logfile is not originally
    present on the system, this interval is how often the existence of
    the logfile will be checked for. The default of 5 seconds is
    usually fine. This interval is in milliseconds.
- stat_interval (int):
    How often the file descriptors for each file should be checked to
    see if new log data has been written. Defaults to 500 milliseconds.
    This interval is in milliseconds.
- logger (string):
    Each LogfileInput may specify a logger name to use in the case an
    error occurs during processing of a particular line of logging
    text.  By default, the logger name is set to the logfile name.
- use_seek_journal (bool):
    Specifies whether to use a seek journal to keep track of where we are
    in a file to be able to resume parsing from the same location upon
    restart. Defaults to true.
- seek_journal_name (string):
    Name to use for the seek journal file, if one is used. Only refers to
    the file name itself, not the full path; Heka will store all seek
    journals in a `seekjournal` folder relative to the Heka base directory.
    Defaults to a sanitized version of the `logger` value (which itself
    defaults to the filesystem path of the input file). This value is
    ignored if `use_seek_journal` is set to false.
- resume_from_start (bool):
    When heka restarts, if a logfile cannot safely resume reading from
    the last known position, this flag will determine whether hekad
    will force the seek position to be 0 or the end of file. By
    default, hekad will resume reading from the start of file.

.. versionadded:: 0.4

- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the parsed data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the log on a byte delimiter (default).
    - regexp - splits the log on a regexp delimiter.
    - message.proto - splits the log on protobuf message boundaries
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the log line depending on the delimiter_location configuration.
    Note: when a start delimiter is used the last line in the file will not be
    processed (since the next record defines its end) until the log is rolled.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of a log line.
    - end - the regexp delimiter occurs at the end of the log line (default).

.. code-block:: ini

    [LogfileInput]
    logfile = "/var/log/opendirectoryd.log"
    logger = "opendirectoryd"

.. code-block:: ini

    [LogfileInput]
    logfile = "/var/log/opendirectoryd.log"

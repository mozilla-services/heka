
ProcessInput
============

Executes one or more external programs on an interval, creating
messages from the output.  If a chain of commands is used, stdout is
piped into the next command's stdin. In the event the program returns a
non-zero exit code, ProcessInput will stop, logging the exit error.

Each command is defined with the following parameters:


- name (string):
    Each ProcessInput *must* have a name defined for logging purposes. The
    messages will be tagged with `name`.stdout or `name`.stderr in the
    `ProcessInputName` field of the heka message.
- command (map[uint]cmd_config):
    The command is a structure that contains the full path to the binary,
    command line arguments, optional enviroment variables and an optional
    working directory. See the :ref:`cmd_config` definition below.
    ProcessInput expects the commands to be indexed by integers starting with
    0.
- ticker_interval (uint):
    The number of seconds to wait between runnning `command`.  Defaults to 15.
    A ticker_interval of 0 indicates that the command is run once.
- stdout (bool):
    Capture stdout from `command`.  Defaults to true.
- stderr (bool):
    Capture stderr from `command`.  Defaults to false.
- decoder (string):
    Name of the decoder instance to send messages to.  Default is to inject
    messages back into the main heka router.
- parser_type (string):
    - token - splits the log on a byte delimiter (default).
    - regexp - splits the log on a regexp delimiter.
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the
    start or end of the log line depending on the delimiter_location
    configuration. Note: when a start delimiter is used the last line in the
    file will not be processed (since the next record defines its end) until
    the log is rolled.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of a log line.
    - end - the regexp delimiter occurs at the end of the log line (default).
- timeout (uint):
    Timeout in seconds before any one of the commands in the chain is
    terminated.
- trim (bool) :
    Trim a single trailing newline character if one exists. Default is true.

.. _config_cmd_config:

cmd_config structure:

- bin (string):
    The full path to the binary that will be executed.
- args ([]string):
    Command line arguments to pass into the executable.
- environment ([]string):
    Used to set environment variables before `command` is run. Default is nil,
    which uses the heka process's environment.
- directory (string):
    Used to set the working directory of `Bin` Default is "", which
    uses the heka process's working directory.

.. code-block:: ini

    [ProcessInput]
    name = "DemoProcessInput"
    ticker_interval = 2
    parser_type = "token"
    delimiter = " "
    stdout = true
    stderr = false
    trim = true

    [ProcessInput.command.0]
    bin = "/bin/cat"
    args = ["../testsupport/process_input_pipes_test.txt"]

    [ProcessInput.command.1]
    bin = "/usr/bin/grep"
    args = ["ignore"]

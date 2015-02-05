ProcessInput
============

Executes one or more external programs on an interval, creating messages from
the output.  Supports a chain of commands, where stdout from each process will
be piped into the stdin for the next process in the chain. In the event the
program returns a non-zero exit code, ProcessInput will log that an error
occurred.

Config:

- command (map[uint]cmd_config):
    The command is a structure that contains the full path to the binary,
    command line arguments, optional enviroment variables and an optional
    working directory (see below). ProcessInput expects the commands to be
    indexed by integers starting with 0, where 0 is the first process in the
    chain.
- ticker_interval (uint):
    The number of seconds to wait between each run of `command`.  Defaults to
    15. A ticker_interval of 0 indicates that the command is run only once,
    and should only be used for long running processes that do not exit. If
    ticker_interval is set to 0 and the process exits, then the ProcessInput
    will exit, invoking the restart behavior (see
    :ref:`configuring_restarting`).
- immediate_start (bool):
    If true, heka starts process immediately instead of waiting for first interval defined by ticker_interval to pass.
    Defaults to false.
- stdout (bool):
    If true, for each run of the process chain a message will be generated
    with the last command in the chain's stdout as the payload. Defaults to
    true.
- stderr (bool):
    If true, for each run of the process chain a message will be generated
    with the last command in the chain's stderr as the payload. Defaults to
    false.
- timeout (uint):
    Timeout in seconds before any one of the commands in the chain is
    terminated.
- trim (bool) :
    Trim a single trailing newline character if one exists. Default is true.
- retries (RetryOptions, optional):
    A sub-section that specifies the settings to be used for restart behavior.
    See :ref:`configuring_restarting`

.. _config_cmd_config:

cmd_config structure:

- bin (string):
    The full path to the binary that will be executed.
- args ([]string):
    Command line arguments to pass into the executable.
- env ([]string):
    Used to set environment variables before `command` is run. Default is nil,
    which uses the heka process's environment.
- directory (string):
    Used to set the working directory of `Bin` Default is "", which
    uses the heka process's working directory.

Example:

.. code-block:: ini

    [on_space]
    type = "TokenSplitter"
    delimiter = " "

    [DemoProcessInput]
    type = "ProcessInput"
    ticker_interval = 2
    splitter = "on_space"
    stdout = true
    stderr = false
    trim = true

        [DemoProcessInput.command.0]
        bin = "/bin/cat"
        args = ["../testsupport/process_input_pipes_test.txt"]

        [DemoProcessInput.command.1]
        bin = "/usr/bin/grep"
        args = ["ignore"]

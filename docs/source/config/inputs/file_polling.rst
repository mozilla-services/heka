
FilePollingInput
================

.. versionadded:: 0.7

FilePollingInputs periodically read (unbuffered) the contents of a file
specified, and creates a Heka message with the contents of the file as the
payload.

Config:

- file_path(string):
    The absolute path to the file which the input should read.

- ticker_interval (unit):
    How often, in seconds to input should read the contents of the file.

- decoder (string):
    The name of the decoder used to process the payload of the input.

Example:

.. code-block:: ini

    [MemStats]
    type = "FilePollingInput"
    ticker_interval = 1
    file_path = "/proc/meminfo"
    decoder = "MemStatsDecoder"


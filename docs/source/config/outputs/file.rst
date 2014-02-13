
FileOutput
==========

Writes message data out to a file system.

Parameters:

- path (string):
    Full path to the output file.
- format (string, optional):
    Output format for the message to be written. Supports `json` or
    `protobufstream`, both of which will serialize the entire `Message`
    struct, or `text`, which will output just the payload string. Defaults to
    ``text``.
- prefix_ts (bool, optional):
    Whether a timestamp should be prefixed to each message line in the file.
    Defaults to ``false``.
- perm (string, optional):
    File permission for writing. A string of the octal digit representation.
    Defaults to "644".

Example:

.. code-block:: ini

    [counter_file]
    type = "FileOutput"
    message_matcher = "Type == 'heka.counter-output'"
    path = "/var/log/heka/counter-output.log"
    prefix_ts = true
    perm = "666"

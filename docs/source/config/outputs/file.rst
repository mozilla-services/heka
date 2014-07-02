
FileOutput
==========

Writes message data out to a file system.

Config:

- path (string):
    Full path to the output file.
- perm (string, optional):
    File permission for writing. A string of the octal digit representation.
    Defaults to "644".
- folder_perm (string, optional):
    Permissions to apply to directories created for FileOutput's parent
    directory if it doesn't exist.  Must be a string representation of an
    octal integer. Defaults to "700".
- flush_interval (uint32, optional):
    Interval at which accumulated file data should be written to disk, in
    milliseconds (default 1000, i.e. 1 second). Set to 0 to disable.
- flush_count (uint32, optional):
    Number of messages to accumulate until file data should be written to disk
    (default 1, minimum 1).
- flush_operator (string, optional):
    Operator describing how the two parameters "flush_interval" and
    "flush_count" are combined. Allowed values are "AND" or "OR" (default is
    "AND").
- use_framing (bool, optional):
    Specifies whether or not the encoded data sent out over the TCP connection
    should be delimited by Heka's :ref:`stream_framing`. Defaults to ``true``.

Example:

.. code-block:: ini

    [counter_file]
    type = "FileOutput"
    message_matcher = "Type == 'heka.counter-output'"
    path = "/var/log/heka/counter-output.log"
    perm = "666"
    flush_count = 100
    flush_operator = "OR"
    encoder = "ProtobufEncoder"

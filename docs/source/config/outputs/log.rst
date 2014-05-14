
LogOutput
=========

Logs messages to stdout using Go's `log` package.

Config:

- encoder (string, required):
    .. versionadded:: 0.6

    Encoder plugin used to format the output.

- payload_only (bool, optional):
    .. deprecated:: 0.6
        Use encoder instead.

    If set to true, then only the message payload string will be output,
    otherwise the entire `Message` struct will be output in human readable
    text format.

Example:

.. code-block:: ini

    [counter_output]
    type = "LogOutput"
    message_matcher = "Type == 'heka.counter-output'"
    encoder = "PayloadEncoder"

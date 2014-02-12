
LogOutput
=========

Logs messages to stdout using Go's `log` package.

Parameters:

- payload_only (bool, optional):
    If set to true, then only the message payload string will be output,
    otherwise the entire `Message` struct will be output in JSON format.

Example:

.. code-block:: ini

    [counter_output]
    type = "LogOutput"
    message_matcher = "Type == 'heka.counter-output'"
    payload_only = true

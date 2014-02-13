
CounterFilter
=============

Once a second a `CounterFilter` will generate a message of type `heka.counter-
output`. The payload will contain text indicating the number of messages that
matched the filter's `message_matcher` value during that second (i.e. it
counts the messages the plugin received). Every ten seconds an extra message
(also of type `heka.counter-output`) goes out, containing an aggregate count
and average per second throughput of messages received.

Parameters: **None**

Example:

.. code-block:: ini

    [CounterFilter]
    message_matcher = "Type != 'heka.counter-output'"

.. _config_counter_filter:

Counter Filter
==============

Plugin Name: **CounterFilter**

Once per ticker interval a CounterFilter will generate a message of type `heka
.counter-output`. The payload will contain text indicating the number of
messages that matched the filter's `message_matcher` value during that
interval (i.e. it counts the messages the plugin received). Every ten
intervals an extra message (also of type `heka.counter-output`) goes out,
containing an aggregate count and average per second throughput of messages
received.

Config:

- ticker_interval (int, optional):
	Interval between generated counter messages, in seconds. Defaults to 5.

Example:

.. code-block:: ini

    [CounterFilter]
    message_matcher = "Type != 'heka.counter-output'"

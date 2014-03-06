
StatsdInput
===========

Listens for `statsd protocol <https://github.com/b/statsd_spec>`_ `counter`,
`timer`, or `gauge` messages on a UDP port, and generates `Stat` objects that
are handed to a `StatAccumulator` for aggregation and processing.

Config:

- address (string):
    An IP address:port on which this plugin will expose a statsd server.
    Defaults to "127.0.0.1:8125".
- stat_accum_name (string):
    Name of a StatAccumInput instance that this StatsdInput will use as its
    StatAccumulator for submitting received stat values. Defaults to
    "StatAccumInput".

Example:

.. code-block:: ini

    [StatsdInput]
    address = ":8125"
    stat_accum_input = "custom_stat_accumulator"

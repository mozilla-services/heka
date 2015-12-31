.. _config_stat_accum_input:

Stat Accumulator Input
======================

Plugin Name: **StatAccumInput**

Provides an implementation of the `StatAccumulator` interface which other
plugins can use to submit `Stat` objects for aggregation and roll-up.
Accumulates these stats and then periodically emits a "stat metric" type
message containing aggregated information about the stats received since the
last generated message.

Config:

- emit_in_payload (bool):
    Specifies whether or not the aggregated stat information should be emitted
    in the payload of the generated messages, in the format accepted by the
    `carbon <http://graphite.wikidot.com/carbon>`_ portion of the `graphite
    <http://graphite.wikidot.com/>`_ graphing software. Defaults to true.
- emit_in_fields (bool):
    Specifies whether or not the aggregated stat information should be emitted
    in the message fields of the generated messages. Defaults to false. *NOTE*:
    At least one of 'emit_in_payload' or 'emit_in_fields' *must* be true or it
    will be considered a configuration error and the input won't start.
- percent_threshold (slice):
    Percent threshold to use for computing "upper_N%" type stat values.
    Defaults to [90].
- ticker_interval (uint):
    Time interval (in seconds) between generated output messages.
    Defaults to 10.
- message_type (string):
    String value to use for the `Type` value of the emitted stat messages.
    Defaults to "heka.statmetric".
- legacy_namespaces (bool):
    If set to true, then use the older format for namespacing counter stats,
    with rates recorded under `stats.<counter_name>` and absolute count
    recorded under `stats_counts.<counter_name>`. See `statsd metric
    namespacing
    <https://github.com/etsy/statsd/blob/master/docs/namespacing.md>`_.
    Defaults to false.
- global_prefix (string):
    Global prefix to use for sending stats to graphite. Defaults to "stats".
- counter_prefix (string):
    Secondary prefix to use for namespacing counter metrics. Has no impact
    unless `legacy_namespaces` is set to false. Defaults to "counters".
- timer_prefix (string):
    Secondary prefix to use for namespacing timer metrics. Defaults to
    "timers".
- gauge_prefix (string):
    Secondary prefix to use for namespacing gauge metrics. Defaults to
    "gauges".
- statsd_prefix (string):
    Prefix to use for the statsd `numStats` metric. Defaults to "statsd".
- delete_idle_stats (bool):
    Don't emit values for inactive stats instead of sending 0 or in the case
    of gauges, sending the previous value. Defaults to false.

Example:

.. code-block:: ini

    [StatAccumInput]
    emit_in_fields = true
    delete_idle_stats = true
    ticker_interval = 5

.. _config_stat_filter:

Stat Filter
===========

Plugin Name: **StatFilter**

Filter plugin that accepts messages of a specfied form and uses extracted
message data to feed statsd-style numerical metrics in the form of `Stat`
objects to a `StatAccumulator`.

Config:

- Metric:

    Subsection defining a single metric to be generated. Both the `name` and
    `value` fields for each metric support interpolation of message field
    values (from 'Type', 'Hostname', 'Logger', 'Payload',  or any dynamic
    field name) with the use of %% delimiters, so `%Hostname%` would be
    replaced by the message's Hostname field, and %Foo% would be replaced by
    the first value of a dynamic field called "Foo":

    - type (string):
        Metric type, supports "Counter", "Timer", "Gauge".
    - name (string):
        Metric name, must be unique.
    - value (string):
        Expression representing the (possibly dynamic) value that the
        `StatFilter` should emit for each received message.
    - replace_dot (boolean):
        Replace all dots `.` per an underscore `_` during the string
        interpolation. It's useful if you output this result in a graphite
        instance.

- stat_accum_name (string):
    Name of a StatAccumInput instance that this StatFilter will use as its
    StatAccumulator for submitting generate stat values. Defaults to
    "StatAccumInput".

Example:

.. code-block:: ini

    [StatAccumInput]
    ticker_interval = 5

    [StatsdInput]
    address = "127.0.0.1:29301"

    [Hits]
    type = "StatFilter"
    message_matcher = 'Type == "ApacheLogfile"'

    [Hits.Metric.bandwidth]
    type = "Counter"
    name = "httpd.bytes.%Hostname%"
    value = "%Bytes%"

    [Hits.Metric.method_counts]
    type = "Counter"
    name = "httpd.hits.%Method%.%Hostname%"
    value = "1"

.. note::

    StatFilter requires an available StatAccumInput to be running.

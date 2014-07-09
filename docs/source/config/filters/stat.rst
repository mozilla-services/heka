
StatFilter
==========

Filter plugin that accepts messages of a specfied form and uses extracted
message data to generate statsd-style numerical metrics in the form of `Stat`
objects that can be consumed by a `StatAccumulator`.

Config:

- Metric:
    Subsection defining a single metric to be generated:

    - type (string):
        Metric type, supports "Counter", "Timer", "Gauge".
    - name (string):
        Metric name, must be unique.
    - value (string):
        Expression representing the (possibly dynamic) value that the
        `StatFilter` should emit for each received message.

- stat_accum_name (string):
    Name of a StatAccumInput instance that this StatFilter will use as its
    StatAccumulator for submitting generate stat values. Defaults to
    "StatAccumInput".

Example (Assuming you had TransformFilter inserting messages as above):

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

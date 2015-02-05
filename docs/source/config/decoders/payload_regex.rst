.. _config_payloadregex_decoder:

Payload Regex Decoder
=====================

Plugin Name: **PayloadRegexDecoder**

Decoder plugin that accepts messages of a specified form and generates new
outgoing messages from extracted data, effectively transforming one message
format into another.

.. note::

    The `Go regular expression tester <https://regoio.herokuapp.com/>`_ is an
    invaluable tool for constructing and debugging regular expressions to be
    used for parsing your input data.

Config:

- match_regex:
    Regular expression that must match for the decoder to process the message.
- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
    See :ref:`message`.
- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in a regex
    in the message_matcher, and any other field that exists in the message. In
    the event that a captured name overlaps with a message field, the captured
    name's value will be used. Optional representation metadata can be added at
    the end of the field name using a pipe delimiter i.e. ResponseSize|B  =
    "%ResponseSize%" will create Fields[ResponseSize] representing the number of
    bytes.  Adding a representation string to a standard message header name
    will cause it to be added as a user defined field i.e., Payload|json will
    create Fields[Payload] with a json representation
    (see :ref:`field_variables`).

    Interpolated values should be surrounded with `%` signs, for example::

        [my_decoder.message_fields]
        Type = "%Type%Decoded"

    This will result in the new message's Type being set to the old messages
    Type with `Decoded` appended.
- timestamp_layout (string):
    A formatting string instructing hekad how to turn a time string into the
    actual time representation used internally. Example timestamp layouts can
    be seen in `Go's time documentation <http://golang.org/pkg/time/#pkg-
    constants>`_. In addition to the Go time formatting, special
    `timestamp_layout` values of "Epoch", "EpochMilli", "EpochMicro", and
    "EpochNano" are supported for Unix style timestamps represented in
    seconds, milliseconds, microseconds, and nanoseconds since the Epoch,
    respectively.
- timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`. This setting will
    have no impact if one of the supported "Epoch*" values is used as the
    `timestamp_layout` setting.
- log_errors (bool):
    .. versionadded:: 0.5

    If set to false, payloads that can not be matched against the regex will
    not be logged as errors. Defaults to true.

Example (Parsing Apache Combined Log Format):

.. code-block:: ini

    [apache_transform_decoder]
    type = "PayloadRegexDecoder"
    match_regex = '^(?P<RemoteIP>\S+) \S+ \S+ \[(?P<Timestamp>[^\]]+)\] "(?P<Method>[A-Z]+) (?P<Url>[^\s]+)[^"]*" (?P<StatusCode>\d+) (?P<RequestSize>\d+) "(?P<Referer>[^"]*)" "(?P<Browser>[^"]*)"'
    timestamp_layout = "02/Jan/2006:15:04:05 -0700"

    # severities in this case would work only if a (?P<Severity>...) matching
    # group was present in the regex, and the log file contained this information.
    [apache_transform_decoder.severity_map]
    DEBUG = 7
    INFO = 6
    WARNING = 4

    [apache_transform_decoder.message_fields]
    Type = "ApacheLogfile"
    Logger = "apache"
    Url|uri = "%Url%"
    Method = "%Method%"
    Status = "%Status%"
    RequestSize|B = "%RequestSize%"
    Referer = "%Referer%"
    Browser = "%Browser%"

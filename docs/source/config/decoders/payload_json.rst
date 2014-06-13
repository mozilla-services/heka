
PayloadJsonDecoder
==================

**NOTE**: The PayloadJsonDecoder is deprecated. The recommended way to parse
JSON is to make use of the cjson Lua library in a
:ref:`config_sandboxdecoder`. This approach is typically both faster and more
flexible than using PayloadJsonDecoder.

This decoder plugin accepts JSON blobs and allows you to map parts
of the JSON into Field attributes of the pipeline pack message using
JSONPath syntax.

Config:

- json_map:
    A subsection defining a capture name that maps to a JSONPath expression.
    Each expression can fetch a single value, if the expression does
    not resolve to a valid node in the JSON message, the capture group
    will be assigned an empty string value.

- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
    See :ref:`message`.

- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in a JSONPath
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
    constants>`_.  The default layout is ISO8601 - the same as
    Javascript.

- timestamp_location (string):
    Time zone in which the timestamps in the text are presumed to be in.
    Should be a location name corresponding to a file in the IANA Time Zone
    database (e.g. "America/Los_Angeles"), as parsed by Go's
    `time.LoadLocation()` function (see
    http://golang.org/pkg/time/#LoadLocation). Defaults to "UTC". Not required
    if valid time zone info is embedded in every parsed timestamp, since those
    can be parsed as specified in the `timestamp_layout`.

- require_all_fields (bool):
    Requires json mappings to match ALL specified message_fields. If any
    message field is not matched, the pack is silently passed back unmodified.
    Useful when used in combination with a MultiDecoder and multiple JSON
    formats. Defaults to "false"

    Example:

    .. code-block:: ini

        [my_multi_decoder]
        type = "MultiDecoder"
        subs = [log1_json", "log2_json"]
        cascade_strategy = "all"

        [log1_json]
        type = "PayloadJsonDecoder"
        require_all_fields = "true"

            [log1_json.json_map]
            field1 = "$.field1"
            field2 = "$.field2"
            field3 = "$.field3"

            [log1_json.message_fields]
            field1 = "%field1%"
            field2 = "%field2%"
            fieddld3 = "%field3%"

        [log2_json]
        type = "PayloadJsonDecoder"
        require_all_fields = "true"

            [log2_json.json_map]
            field4 = "$.field4"
            field5 = "$.field5"
            field6 = "$.field6"

            [log2_json.message_fields]
            field4 = "%field4%"
            field5 = "%field5%"
            field6 = "%field6%"

Example:

.. code-block:: ini

    [myjson_decoder]
    type = "PayloadJsonDecoder"

        [myjson_decoder.json_map]
        Count = "$.statsd.count"
        Name = "$.statsd.name"
        Pid = "$.pid"
        Timestamp = "$.timestamp"
        Severity = "$.log_level"

        [myjson_decoder.severity_map]
        DEBUG = 7
        INFO = 6
        WARNING = 4

        [myjson_decoder.message_fields]
        Pid = "%Pid%"
        StatCount = "%Count%"
        StatName =  "%Name%"
        Timestamp = "%Timestamp%"

PayloadJsonDecoder's json_map config subsection only supports a small
subset of valid JSONPath expressions.

========     =========================================
JSONPath     Description
========     =========================================
$            the root object/element
.            child operator
[]           subscript operator to iterate over arrays
========     =========================================

Examples:
---------

.. code-block:: javascript

    var s = {
        "foo": {
            "bar": [
                {
                    "baz": "こんにちわ世界",
                    "noo": "aaa"
                },
                {
                    "maz": "123",
                    "moo": 256
                }
            ],
            "boo": {
                "bag": true,
                "bug": false
            }
        }
    }

    # Valid paths
    $.foo.bar[0].baz
    $.foo.bar

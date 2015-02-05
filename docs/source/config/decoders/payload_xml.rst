.. _config_payload_xml_decoder:

Payload XML Decoder
===================

Plugin Name: **PayloadXmlDecoder**

This decoder plugin accepts XML blobs in the message payload and allows you to
map parts of the XML into Field attributes of the pipeline pack message using
XPath syntax using the `xmlpath <http://launchpad.net/xmlpath>`_ library.

Config:

- xpath_map:
    A subsection defining a capture name that maps to an XPath expression.
    Each expression can fetch a single value, if the expression does
    not resolve to a valid node in the XML blob, the capture group
    will be assigned an empty string value.
- severity_map:
    Subsection defining severity strings and the numerical value they should
    be translated to. hekad uses numerical severity codes, so a severity of
    `WARNING` can be translated to `3` by settings in this section.
    See :ref:`message`.
- message_fields:
    Subsection defining message fields to populate and the interpolated values
    that should be used. Valid interpolated values are any captured in an XPath
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
    constants>`_.  The default layout is ISO8601 - the same as Javascript. In
    addition to the Go time formatting, special `timestamp_layout` values of
    "Epoch", "EpochMilli", "EpochMicro", and "EpochNano" are supported for
    Unix style timestamps represented in seconds, milliseconds, microseconds,
    and nanoseconds since the Epoch, respectively.
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

Example:

.. code-block:: ini

    [myxml_decoder]
    type = "PayloadXmlDecoder"

    [myxml_decoder.xpath_map]
    Count = "/some/path/count"
    Name = "/some/path/name"
    Pid = "//pid"
    Timestamp = "//timestamp"
    Severity = "//severity"

    [myxml_decoder.severity_map]
    DEBUG = 7
    INFO = 6
    WARNING = 4

    [myxml_decoder.message_fields]
    Pid = "%Pid%"
    StatCount = "%Count%"
    StatName =  "%Name%"
    Timestamp = "%Timestamp%"

PayloadXmlDecoder's xpath_map config subsection supports XPath as
implemented by the `xmlpath <http://launchpad.net/xmlpath>`_ library.

    * All axes are supported ("child", "following-sibling", etc)
    * All abbreviated forms are supported (".", "//", etc)
    * All node types except for namespace are supported
    * Predicates are restricted to [N], [path], and [path=literal] forms
    * Only a single predicate is supported per path step
    * Richer expressions and namespaces are not supported

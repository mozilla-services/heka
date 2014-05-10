
MultiDecoder
============

This decoder plugin allows you to specify an ordered list of delegate
decoders.  The MultiDecoder will pass the PipelinePack to be decoded to each
of the delegate decoders in turn until decode succeeds.  In the case of
failure to decode, MultiDecoder will return an error and recycle the message.

Config:

- subs:
    A subsection is used to declare the TOML configuration for any delegate
    decoders. The default is that no delegate decoders are defined.

- order (list of strings):
    PipelinePack objects will be passed in order to each decoder in this list.

- log_sub_errors (bool):
    If true, the DecoderRunner will log the errors returned whenever a
    delegate decoder fails to decode a message. Defaults to false.

- cascade_strategy (string):
    Specifies behavior the MultiDecoder should exhibit with regard to
    cascading through the listed decoders. Supports only two valid values:
    "first-wins" and "all". With "first-wins", each decoder will be tried in
    turn until there is a successful decoding, after which decoding will be
    stopped. With "all", all listed decoders will be applied whether or not
    they succeed. In each case, decoding will only be considered to have
    failed if *none* of the sub-decoders succeed.

Example (Two PayloadRegexDecoder delegates):

.. code-block:: ini

        [syncdecoder]
        type = "MultiDecoder"
        order = ['syncformat', 'syncraw']

        [syncdecoder.subs.syncformat]
        type = "PayloadRegexDecoder"
        match_regex = '^(?P<RemoteIP>\S+) \S+ (?P<User>\S+) \[(?P<Timestamp>[^\]]+)\] "(?P<Method>[A-Z]+) (?P<Url>[^\s]+)[^"]*" (?P<StatusCode>\d+) (?P<RequestSize>\d+) "(?P<Referer>[^"]*)" "(?P<Browser>[^"]*)" ".*" ".*" node_s:\d+\.\d+ req_s:(?P<ResponseTime>\d+\.\d+) retries:\d+ req_b:(?P<ResponseSize>\d+)'
        timestamp_layout = "02/Jan/2006:15:04:05 -0700"

        [syncdecoder.subs.syncformat.message_fields]
        RemoteIP|ipv4 = "%RemoteIP%"
        User = "%User%"
        Method = "%Method%"
        Url|uri = "%Url%"
        StatusCode = "%StatusCode%"
        RequestSize|B= "%RequestSize%"
        Referer = "%Referer%"
        Browser = "%Browser%"
        ResponseTime|s = "%ResponseTime%"
        ResponseSize|B = "%ResponseSize%"
        Payload = ""

        [syncdecoder.subs.syncraw]
        type = "PayloadRegexDecoder"
        match_regex = '^(?P<TheData>.*)'

        [syncdecoder.subs.syncraw.message_fields]
        Somedata = "%TheData%"

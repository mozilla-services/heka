.. _config_irc_output:

IRC Output
==========

Plugin Name: **IrcOutput**

Connects to an Irc Server and sends messages to the specified Irc channels.
Output is encoded using the specified encoder, and expects output to be properly
truncated to fit within the bounds of an Irc message before being receiving the
output.

Config:

- server (string):
    A host:port of the irc server that Heka will connect to for sending output.
- nick (string):
    Irc nick used by Heka.
- ident (string):
    The Irc identity used to login with by Heka.
- password (string, optional):
    The password used to connect to the Irc server.
- channels (list of strings):
    A list of Irc channels which every matching Heka message is sent to. If
    there is a space in the channel string, then the part after the space is
    expected to be a password for a protected irc channel.
- timeout (uint, optional):
    The maximum amount of time (in seconds) to wait before timing out when
    connect, reading, or writing to the Irc server. Defaults to 10.
- tls (TlsConfig, optional):
    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.
- queue_size (uint, optional):
    This is the maximum amount of messages Heka will queue per Irc channel
    before discarding messages. There is also a queue of the same size used
    if all per-irc channel queues are full. This is used when Heka is unable to
    send a message to an Irc channel, such as when it hasn't joined or has been
    disconnected. Defaults to 100.
- rejoin_on_kick (bool, optional):
    Set this if you want Heka to automatically re-join an Irc channel after being
    kicked. If not set, and Heka is kicked, it will not attempt to rejoin ever.
    Defaults to false.
- ticker_interval (uint, optional):
    How often (in seconds) heka should send a message to the server. This is
    on a per message basis, not per channel. Defaults to 2.
- time_before_reconnect (uint, optional):
    How long to wait (in seconds) before reconnecting to the Irc server after
    being disconnected. Defaults to 3.
- time_before_rejoin (uint, optional):
    How long to wait (in seconds) before attempting to rejoin an Irc channel
    which is full. Defaults to 3.
- max_join_retries (uint, optional):
    The maximum amount of attempts Heka will attempt to join an Irc channel
    before giving up. After attempts are exhausted, Heka will no longer attempt
    to join the channel. Defaults to 3.
- verbose_irc_logging (bool, optional):
    Enable to see raw internal message events Heka is receiving from the server.
    Defaults to false.
- encoder (string):
    Specifies which of the registered encoders should be used for converting
    Heka messages into what is sent to the irc channels.
- retries (RetryOptions, optional):
    A sub-section that specifies the settings to be used for restart behavior.
    See :ref:`configuring_restarting`


Example:

.. code-block:: ini

    [IrcOutput]
    message_matcher = 'Type == "alert"'
    encoder = "PayloadEncoder"
    server = "irc.mozilla.org:6667"
    nick = "heka_bot"
    ident = "heka_ident"
    channels = [ "#heka_bot_irc testkeypassword" ]
    rejoin_on_kick = true
    queue_size = 200
    ticker_interval = 1

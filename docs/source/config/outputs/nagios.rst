.. _config_nagios_output:

Nagios Output
=============

Plugin Name: **NagiosOutput**

Specialized output plugin that listens for Nagios external command message
types and delivers passive service check results to Nagios using either HTTP
requests made to the Nagios cmd.cgi API or the use of the `send_ncsa` binary.
The message payload must consist of a state followed by a colon and then the
message e.g., "OK:Service is functioning properly". The valid states are:
OK|WARNING|CRITICAL|UNKNOWN.  Nagios must be configured with a service name
that matches the Heka plugin instance name and the hostname where the plugin
is running.

Config:

- url (string, optional):
    An HTTP URL to the Nagios cmd.cgi. Defaults to
    http://localhost/nagios/cgi-bin/cmd.cgi.
- username (string, optional):
    Username used to authenticate with the Nagios web interface. Defaults to
    empty string.
- password (string, optional):
    Password used to authenticate with the Nagios web interface. Defaults to
    empty string.
- response_header_timeout (uint, optional):
    Specifies the amount of time, in seconds, to wait for a server's response
    headers after fully writing the request. Defaults to 2.
- nagios_service_description (string, optional):
    Must match Nagios service's service_description attribute. Defaults to the
    name of the output.
- nagios_host (string, optional):
    Must match the hostname of the server in nagios. Defaults to the Hostname
    attribute of the message.
- send_nsca_bin (string, optional):
    .. versionadded:: 0.5

    Use send_nsca program, as provided, rather than sending HTTP requests. Not
    supplying this value means HTTP will be used, and any other send_nsca_*
    settings will be ignored.
- send_nsca_args ([]string, optional):
    .. versionadded:: 0.5

    Arguments to use with send_nsca, usually at least the nagios hostname,
    e.g. `["-H", "nagios.somehost.com"]`. Defaults to an empty list.
- send_nsca_timeout (int, optional):
    .. versionadded:: 0.5

    Timeout for the send_nsca command, in seconds. Defaults to 5.
- use_tls (bool, optional):
    .. versionadded:: 0.5

    Specifies whether or not SSL/TLS encryption should be used for the TCP
    connections. Defaults to false.
- tls (TlsConfig, optional):
    .. versionadded:: 0.5

    A sub-section that specifies the settings to be used for any SSL/TLS
    encryption. This will only have any impact if `use_tls` is set to true.
    See :ref:`tls`.

Example configuration to output alerts from SandboxFilter plugins:

.. code-block:: ini

    [NagiosOutput]
    url = "http://localhost/nagios/cgi-bin/cmd.cgi"
    username = "nagiosadmin"
    password = "nagiospw"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'nagios-external-command' && Fields[payload_name] == 'PROCESS_SERVICE_CHECK_RESULT'"

Example Lua code to generate a Nagios alert:

.. code-block:: lua

    inject_payload("nagios-external-command", "PROCESS_SERVICE_CHECK_RESULT", "OK:Alerts are working!")

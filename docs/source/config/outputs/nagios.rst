
NagiosOutput
============

Specialized output plugin that listens for Nagios external command message types
and generates an HTTP request against the Nagios cmd.cgi API. Currently the
output will only send passive service check results.  The message payload must
consist of a state followed by a colon and then the message i.e.,
"OK:Service is functioning properly". The valid states are:
OK|WARNING|CRITICAL|UNKNOWN.  Nagios must be configured with a service name that
matches the Heka plugin instance name and the hostname where the plugin is
running.

Parameters:

- url (string, optional):
    An HTTP URL to the Nagios cmd.cgi. Defaults to "http://localhost/nagios/cgi-bin/cmd.cgi".
- username (string, optional):
    Username used to authenticate with the Nagios web interface. Defaults to "".
- password (string, optional):
    Password used to authenticate with the Nagios web interface. Defaults to "".
- responseheadertimeout (uint, optional):
    Specifies the amount of time, in seconds, to wait for a server's response
    headers after fully writing the request. Defaults to 2.

Example configuration to output alerts from SandboxFilter plugins:

.. code-block:: ini

    [NagiosOutput]
    url = "http://localhost/nagios/cgi-bin/cmd.cgi"
    username = "nagiosadmin"
    password = "nagiospw"
    message_matcher = "Type == 'heka.sandbox-output' && Fields[payload_type] == 'nagios-external-command' && Fields[payload_name] == 'PROCESS_SERVICE_CHECK_RESULT'"

Example Lua code to generate a Nagios alert:

.. code-block:: lua

    output("OK:Alerts are working!")
    inject_message("nagios-external-command", "PROCESS_SERVICE_CHECK_RESULT")

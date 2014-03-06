
SmtpOutput
==========

.. versionadded:: 0.5

Outputs a Heka message in an email.  The message subject is the plugin name
and the message content is controlled by the payload_only setting.  The
primary purpose is for email alert notifications e.g., PagerDuty.

Config:

- payload_only (bool)
    If set to true, then only the message payload string will be emailed,
    otherwise the entire `Message` struct will be emailed in JSON format.
    (default: true)
- send_from (string)
    The email address of the sender. (default: "heka@localhost.localdomain")
- send_to (array of strings)
    An array of email addresses where the output will be sent to.
- subject (string)
    Custom subject line of email. (default: "Heka [SmtpOutput]")
- host (string)
    SMTP host to send the email to (default: "127.0.0.1:25")
- auth (string)
    SMTP authentication type: "none", "Plain", "CRAMMD5" (default: "none")
- user (string, optional)
    SMTP user name
- password (string, optional)
    SMTP user password

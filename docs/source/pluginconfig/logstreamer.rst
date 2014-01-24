.. _logstreamerplugin:

===========
Logstreamer
===========

The Logstreamer plugin scans, sorts, and reads logstreams in a
sequential user-defined order, differentiating multiple logstreams
found in a search based on a user-defined differentiator. This plugin
supercedes the LogfileInput and the LogfileDirectoryInput.

A Logstream is considered to be a single logical stream of events of a
given source of a given type. For example, Apache/nginx can create
multiple logfiles for a single domain. One for access logs, one for
error logs. The system may rotate the logs adding an increasing integer
to the end, (access.log, acces.log.1, access.log.2, etc). A Logstream
could be considered all the access logs for a single domain.

To make it easier to parse multiple logstreams, the Logstreamer plugin
can be specified a single time with a single decoder for all the
logstreams that should be parsed with it.

Configuration
=============

Given the flexibility of the Logstreamer, configuration can be more
complex for the more advanced use-cases. We'll start with the simplest
use-case and work towards the most complex.

Single Rotating Logfile
-----------------------

This is the basic use-case that LogfileInput handled previously, where
a single logfile should be read that the system may rotate/truncate at
some time (hopefully not using truncation though that condition is
handled). Log rotation inherently has a risk that some loglines written
may be missed if the program reading the log happens to die at exactly
the wrong time that the rotation is occuring.

An example of a single rotating logfile would be the case where you
want to watch /var/log/system.log for all new entries. Here's what the
configuration for such a case looks like:

.. code-block:: ini

    [syslog]
    type = "LogstreamerInput"
    file_match = '^/var/log/system.log$'

By default Logstreamer will scan under the ``/var/log`` directory and
attempt to match every file it discovers there against the
``file_match`` pattern. Note that we leave the full path on the front
to indicate that a file like ``/var/log/nginx/system.log`` should not
match. This is because the entire path for every file will be run
against the ``file_match`` pattern so we must match it strictly.

Multiple Single Rotating Logfiles
---------------------------------

This use-case was previously handled by the
LogfileDirectoryManagerInput which is similar to the single rotating
logfile above except there are multiple separate ones with the same
policy.

An example of multiple single rotating logfiles that all happen to be
in the same directory with a domain name marking each one. In this case
to differentiate them, we will need to indicate what part of the
``file_match`` indicates its a separate logfile.

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    file_match = '^/var/log/nginx/(?P<DomainName>.*?)-access.log$'
    differentiator = ["nginx.", "DomainName", ".access"]

Note that we included two strings in the differentiator that don't
correspond to a part in the ``file_match`` regular expression. These
two parts will be included as is to create the logger name attached to
each message. So a file:

``/var/log/nginx/hekathings.com-access.log``

Will have all its messages in heka with the logger name set to
``nginx.hekathings.com.access``.

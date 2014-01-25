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
    file_match = '/var/log/system\.log'

By default Logstreamer will scan under the ``/var/log`` directory and
attempt to match every file it discovers there against the
``file_match`` pattern. Note that we leave the full path on the front
to indicate that a file like ``/var/log/nginx/system.log`` should not
match. This is because the entire path for every file will be run
against the ``file_match`` pattern so we must match it strictly.

.. note:: How it Works

    An option not shown here is the ``log_directory``, which defaults
    to ``/var/log``. Every file under this directory root is located
    with a recursive directory walker and matched against
    ``file_match`` to see if its a logfile.

    Be careful not to specify a log root directory that is too broad or
    startup may take longer as it walks the directory structure.

Multiple Single Rotating Logfiles
---------------------------------

This use-case was previously handled by the
LogfileDirectoryManagerInput which is similar to the single rotating
logfile above except there are multiple separate ones with the same
policy.

An example of multiple single rotating logfiles would be a system that
logs the access for each domain name to a separate access log. In this
case to differentiate them, we will need to indicate what part of the
``file_match`` indicates its a separate logfile (using the domain name
as the differentiator).

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    file_match = '/var/log/nginx/(?P<DomainName>[^/]+)-access\.log'
    differentiator = ["nginx.", "DomainName", ".access"]

Note that we included two strings in the differentiator that don't
correspond to a part in the ``file_match`` regular expression. These
two parts will be included as is to create the logger name attached to
each message. So a file:

``/var/log/nginx/hekathings.com-access.log``

Will have all its messages in heka with the logger name set to
``nginx.hekathings.com.access``.

Single Sequential (Rotating) Logfile
------------------------------------

What happens if you have a log structure like this?

..
    /var/log/nginx/access.log
    /var/log/nginx/access.log.1
    /var/log/nginx/access.log.2
    /var/log/nginx/access.log.3

Or perhaps like this?

..

    /var/log/nginx/2014/08/1.access.log
    /var/log/nginx/2014/08/2.access.log
    /var/log/nginx/2014/08/3.access.log
    /var/log/nginx/2014/08/4.access.log

Or a combination of them?

..

    /var/log/nginx/2014/08/access.log
    /var/log/nginx/2014/08/access.log.1
    /var/log/nginx/2014/08/access.log.2
    /var/log/nginx/2014/08/access.log.3

(Hopefully your setup isn't worse than any of these... but even if it is then Logstreamer can handle it.)

Handling a single access log that is sequential and rotated (the first
example) can be tricky. The second case where rotation doesn't occur
and new logfiles are written every day with new months/years result in
new directories was previously quite difficult to handle. Both of these
cases can be handled by the LogstreamerInput.

The other (fun) problem with the second case is that if you use a raw
string listing of the directory then ``11.access.log`` will come before
``2.access.log`` which is not good if you expect the logs to be in
order.

Let's look at the config for the first case, note that the numbers
incrementing in this case represent the files getting older (the higher
the number, the older the log data):

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    file_match = '/var/log/nginx/access\.log\.?(?P<Seq>\d*)'
    priority = ["^Seq"]

When handling sequential logfiles in a logstream, we need to indicate a
list of matched parts in the ``file_match`` that will be used to sort
the files matching in order from oldest -> newest. By default, the
numbers are sorted in ascending order (which properly reflects oldest
first if the number represents the year, month, or day). To indicate
that we should sort in descending order we use the ``^`` in front of
the matched part to sort on (``Seq``).

Here's what a configuration for the second case:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    file_match = '/var/log/nginx/(?P<Year>\d+)/(?P<Month>\d+)/(?P<Day>\d+)\.access\.log'
    priority = ["Year", "Month", "Day"]

First we match the portions to be sorted on, and then we specify the
priority of matched portions to sort with.

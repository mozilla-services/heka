.. _logstreamerplugin:

===========
Logstreamer
===========

.. versionadded:: 0.5

The Logstreamer plugin scans, sorts, and reads logstreams in a
sequential user-defined order, differentiating multiple logstreams
found in a search based on a user-defined differentiator.

A "logstream" is a single, linear data stream that is spread across
one or more sequential log files. For instance, an Apache or nginx
server typically generates two logstreams for each domain: an access
log and an error log. Each stream might be written to a single log file
that is periodically truncated (ick!) or rotated (better), with some
number of historical versions being kept (e.g. access-example.com.log,
access-example.com.log.0, access-example.com.log.1, etc.). Or, better
yet, the server might periodically create new timestamped files so that
the 'tip' of the logstream jumps from file to file (e.g. access-
example.com-2014.01.28.log, access-example.com-2014.01.27.log, access-
example.com-2014.01.26.log, etc.). The job of Heka's Logstreamer plugin
is to understand the file naming and ordering conventions for a single
type of logstream (e.g. "all of the nginx server's domain access
logs"), and to use that to watch the specified directories and load the
right files in the right order. The plugin will also track its
location in the stream so it can resume from where it left off after a
restart, even in cases where the file may have rotated during the
downtime.

To make it easier to parse multiple logstreams, the Logstreamer plugin
can be specified a single time with a single decoder for all the
logstreams that should be parsed with it.

Standard Configurations
=======================

Given the flexibility of the Logstreamer, configuration can be more
complex for the more advanced use-cases. We'll start with the simplest
use-case and work towards the most complex.

Single Rotating Logfile
-----------------------

This is the basic use-case where a single logfile should be read that the
system may rotate/truncate at some time (hopefully not using truncation though
that condition is handled). Log rotation inherently has a risk that some
loglines written may be missed if the program reading the log happens to die
at exactly the wrong time that the rotation is occuring.

An example of a single rotating logfile would be the case where you
want to watch /var/log/system.log for all new entries. Here's what the
configuration for such a case looks like:

.. code-block:: ini

    [syslog]
    type = "LogstreamerInput"
    log_directory = "/var/log"
    file_match = 'system\.log'

.. note::

    The ``file_match`` config value above is delimited with single quotes
    instead of double quotes (i.e. `'system\\.log'` vs. `"system\\.log"`)
    because single quotes indicate raw strings that do not require backslashes
    to be escaped. If you use double quotes around your regular expressions
    you'll need to escape backslashes by doubling them up, e.g.
    `"system\\\\.log"`.

We start with the highest directory to start scanning for files under, in
this case ``/var/log``. Then the files under that directory (recursively
searching in sub-directories) are matched against the ``file_match``.

The ``log_directory`` should be the most specific directory of files to
match to prevent excessive file scanning to locate the
``file_match``'s.

Multiple Single Rotating Logfiles
---------------------------------

This use-case is similar to the single rotating logfile above except there
are multiple separate files with the same policy.

An example of multiple single rotating logfiles would be a system that
logs the access for each domain name to a separate access log. In this
case to differentiate them, we will need to indicate what part of the
``file_match`` indicates its a separate logfile (using the domain name
as the differentiator).

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<DomainName>[^/]+)-access\.log'
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

.. code-block:: bash

    /var/log/nginx/access.log
    /var/log/nginx/access.log.1
    /var/log/nginx/access.log.2
    /var/log/nginx/access.log.3

Or perhaps like this?

.. code-block:: bash

    /var/log/nginx/2014/08/1.access.log
    /var/log/nginx/2014/08/2.access.log
    /var/log/nginx/2014/08/3.access.log
    /var/log/nginx/2014/08/4.access.log

Or a combination of them?

.. code-block:: bash

    /var/log/nginx/2014/08/access.log
    /var/log/nginx/2014/08/access.log.1
    /var/log/nginx/2014/08/access.log.2
    /var/log/nginx/2014/08/access.log.3

(Hopefully your setup isn't worse than any of these... but even if it is then
Logstreamer can handle it.)

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
    log_directory = "/var/log/nginx"
    file_match = 'access\.log\.?(?P<Seq>\d*)'
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
    log_directory = "/var/log/nginx"
    file_match = '(?P<Year>\d+)/(?P<Month>\d+)/(?P<Day>\d+)\.access\.log'
    priority = ["Year", "Month", "Day"]

First we match the portions to be sorted on, and then we specify the
priority of matched portions to sort with. In this case the lower
numbers represent older data so none of them need to be prefixed with
``^``.

Finally, the last configuration is a mix of the prior two:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<Year>\d+)/(?P<Month>\d+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "Month", "^Seq"]

Multiple Sequential (Rotating) Logfiles
---------------------------------------

Same as before, except now we need to differentiate the sequential
streams. We're only introducing a single parameter here that we've seen
before to handle the differentiation. Lets take the last case from
above and consider it a multiple sequential source.

Example directory layout:

.. code-block:: bash

    /var/log/nginx/frank.com/2014/08/access.log
    /var/log/nginx/frank.com/2014/08/access.log.1
    /var/log/nginx/frank.com/2014/08/access.log.2
    /var/log/nginx/frank.com/2014/08/access.log.3
    /var/log/nginx/george.com/2014/08/access.log
    /var/log/nginx/george.com/2014/08/access.log.1
    /var/log/nginx/george.com/2014/08/access.log.2
    /var/log/nginx/george.com/2014/08/access.log.3
    /var/log/nginx/sally.com/2014/08/access.log
    /var/log/nginx/sally.com/2014/08/access.log.1
    /var/log/nginx/sally.com/2014/08/access.log.2
    /var/log/nginx/sally.com/2014/08/access.log.3

In this case we have multiple sequential logfiles for each domain name
that are incrementing in date along with rotation when a logfile gets
too large (causing rotation of the file within the directory).

Configuration for this case:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<DomainName>[^/]+)/(?P<Year>\d+)/(?P<Month>\d+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "Month", "^Seq"]
    differentiator = ["nginx-", "DomainName", "-access"]

As in the case for a non-sequential logfile, we supply a differentiator
that will be used to file each sequential set of logfiles into a
separate logstream.

.. seealso:: :ref:`Full set of configuration options <config_logstreamer_input>`

String-based Order Mappings
===========================

In the standard configurations above, the assumption has been that any
part matched for sorting will be digit(s). This is because the
Logstreamer by default will attempt to coerce a matched portion used
for sorting into an integer in the event a mapping isn't available.
LogstreamerInput comes with several built-in mappings and allows you to
define your own so that matched parts can be translated to integers for
sorting purposes.

Built-in Mappings
-----------------

There are several special regex grouping names you can use that will
indicate to the LogstreamerInput that a default mapping should be used:

- MonthName:
    English full month name or 3-letter version to the appropriate integer.
- DayName:
    English full day name or 3-letter version to the appropriate integer.

If the last example above looked like this:

.. code-block:: bash

    /var/log/nginx/frank.com/2014/Sep/access.log
    /var/log/nginx/frank.com/2014/Oct/access.log.1
    /var/log/nginx/frank.com/2014/Nov/access.log.2
    /var/log/nginx/frank.com/2014/Dec/access.log.3
    /var/log/nginx/sally.com/2014/Sep/access.log
    /var/log/nginx/sally.com/2014/Oct/access.log.1
    /var/log/nginx/sally.com/2014/Nov/access.log.2
    /var/log/nginx/sally.com/2014/Dec/access.log.3

Using the default mappings would provide us a simple configuration:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<Domain>[^/]+)/(?P<Year>\d+)/(?P<MonthName>\s+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "MonthName", "^Seq"]
    differentiator = ["nginx-", "Domain", "-access"]

LogstreamerInput will translate the 3-letter month names automatically
before sorting (If used in the differentiator, you will still get the
original matched string).

Custom Mappings
---------------

What if your logfiles (for reasons we won't speculate about) happened
to use Pharsi month names but Spanish day names such that it looked
like this?

.. code-block:: bash

    /var/log/nginx/sally.com/2014/Hadukannas/lunes/access.log
    /var/log/nginx/sally.com/2014/Turmar/miercoles/access.log
    /var/log/nginx/sally.com/2014/Karmabatas/jueves/access.log
    /var/log/nginx/sally.com/2014/Karbasiyas/sabado/access.log

It would be easier if the logging scheme just used month and day
integers but changing existing systems isn't always an option, so lets
work with this somewhat odd scheme.

The first chunk of our configuration:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<Domain>[^/]+)/(?P<Year>\d+)/(?P<Month>\s+)/(?P<Day>[^/]+/access\.log'
    priority = ["Year", "Month", "Day"]
    differentiator = ["nginx-", "Domain", "-access"]

Now to supply the important mapping of how to translate ``Month`` and
``Day`` into sortable integers. We'll add this:

.. code-block:: ini

    [accesslogs.translation.Month]
    hadukannas = 1
    turmar = 2
    karmabatas = 4
    karbasiyas = 6

    [accesslogs.translation.Day]
    lunes = 1
    miercoles = 3
    jueves = 4
    sabado = 6

.. note::

    The matched values used are all lowercased before comparison, so 'lunes'
    in the example above would match captured values of 'lunes', 'Lunes', and
    'LuNeS' equivalently.

We left off the rest of the month names and day names not used for
example purposes. Note that if you prefer the week to begin on a
Saturday instead of Monday you can configure it with a custom mapping.

Mappings with Missing Values
----------------------------

In the examples above, the years and months were embedded in the file
path as directory names, but what if the date was embedded into the
filenames themselves, with a file naming schema like so?

.. code-block:: bash

    /var/log/nginx/sally.com/access.log
    /var/log/nginx/sally.com/access-20140803.log
    /var/log/nginx/sally.com/access-20140804.log
    /var/log/nginx/sally.com/access-20140805.log
    /var/log/nginx/sally.com/access-20140806.log
    /var/log/nginx/sally.com/access-20140807.log
    /var/log/nginx/sally.com/access-20140808.log

Notice how the currently active log file contains no date information at all.
As long as you construct your file_match regex correctly this will be fine,
Logstreamer will capture all of the files and won't complain about entries
that are missing the match portions. The following config would work to
capture all of these files:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    log_directory = "/var/log/nginx"
    file_match = '(?P<Domain>[^/]+)/access-?(?P<Year>\d4)(?P<Month>\d2)(?P<Day>\d2)\.log'
    priority = ["Year", "Month", "Day"]
    differentiator = ["nginx-", "Domain", "-access"]

This works to match all of the files because match groups are implicitly
optional and we explicitly made the hyphen separator optional by following it
with a question mark (i.e. `-?`). We still have a problem, however. Heka will
automatically assign a missing match a sort value of -1. Because we're sorting
by date values, which sort naturally in ascending order, the -1 value will
come before every other value, it will be considered the oldest file in the
stream. This is clearly incorrect, since the currently active file is actually
the newest file in the stream.

It is possible to fix this by using a custom translation map to explicitly
associate a sort index with the 'missing' value, like so:

.. code-block:: ini

    [accesslogs.translation.Year]
    missing = 9999

.. note::

    If you create a translation map with only one key, that key *must* be
    'missing'. It's possible to use the 'missing' value in a translation map
    that also contains other keys, but if you have any other key in the map
    you must include *all* possible match values, or else Heka will raise an
    error when it finds a match value that can't be converted.

Verifying Settings
==================

Given the configuration complexity for more advanced use-cases, the
Logstreamer includes a command line tool that lets you verify options
and shows you what logstreams were found, the name, and the order
they'll be parsed in. For convenience the same heka toml config file
may be passed in to ``heka-logstreamer`` and ``LogstreamerInput``
sections will be located and parsed showing you how they were
interpreted.

An example configuration that locates logfiles on an OSX system:

.. code-block:: ini

    [osx-logfiles]
    type = "LogstreamerInput"
    log_directory = "/var/log"
    file_match = '(?P<FileName>[^/]+).log'
    differentiator = ["osx-", "FileName", "-logs"]

Running this through ``heka-logstreamer`` shows the following:

.. code-block:: bash

    $ heka-logstreamer -config=test.toml
    Found 10 Logstream(s) for section [osx-logfiles].

    Logstream name: osx-appstore-logs
    Files: 1 (printing oldest to newest)
        /var/log/appstore.log

    .... more output ....

    Logstream name: osx-bookstore-logs
    Files: 1 (printing oldest to newest)
        /var/log/bookstore.log

    Logstream name: osx-install-logs
    Files: 1 (printing oldest to newest)
        /var/log/install.log

It's recommended to always run ``heka-logstreamer`` first to ensure the
configuration behaves as desired.

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

Base Configuration Options
==========================

A base set of configurations are optional for all the other standard
configurations of the Logstreamer.

Parameters
----------

- hostname (string):
    The hostname to use for the messages, by default this will be the
    machines qualified hostname. This can be set explicitly to ensure
    its the correct name in the event the machine has multiple
    interfaces/hostnames.
- oldest_duration (string):
    A duration as appropriate for Go's duration parser. Logfiles with a
    last modified time older than ``older_duration`` ago will not be included
    for parsing.
- journal_directory (string):
    The directory to store the journal files in for tracking the location that
    has been read to thus far. By default this is stored under heka's base
    directory.
- log_directory (string):
    The root directory to scan files from. This scan is recursive so it
    should be suitably restricted to the most specific directory this
    selection of logfiles will be matched under.
- rescan_interval (int):
    During logfile rotation, or if the logfile is not originally
    present on the system, this interval is how often the existence of
    the logfile will be checked for. The default of 5 seconds is
    usually fine. This interval is in milliseconds.
- decoder (string):
    A :ref:`config_protobuf_decoder` instance must be specified for the
    message.proto parser. Use of a decoder is optional for token and regexp
    parsers; if no decoder is specified the parsed data is available in the
    Heka message payload.
- parser_type (string):
    - token - splits the log on a byte delimiter (default).
    - regexp - splits the log on a regexp delimiter.
    - message.proto - splits the log on protobuf message boundaries
- delimiter (string): Only used for token or regexp parsers.
    Character or regexp delimiter used by the parser (default "\\n").  For the
    regexp delimiter a single capture group can be specified to preserve the
    delimiter (or part of the delimiter). The capture will be added to the start
    or end of the log line depending on the delimiter_location configuration.
    Note: when a start delimiter is used the last line in the file will not be
    processed (since the next record defines its end) until the log is rolled.
- delimiter_location (string): Only used for regexp parsers.
    - start - the regexp delimiter occurs at the start of a log line.
    - end - the regexp delimiter occurs at the end of the log line (default).


Standard Configurations
=======================

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

.. code-block::

    /var/log/nginx/access.log
    /var/log/nginx/access.log.1
    /var/log/nginx/access.log.2
    /var/log/nginx/access.log.3

Or perhaps like this?

.. code-block::

    /var/log/nginx/2014/08/1.access.log
    /var/log/nginx/2014/08/2.access.log
    /var/log/nginx/2014/08/3.access.log
    /var/log/nginx/2014/08/4.access.log

Or a combination of them?

.. code-block::

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
priority of matched portions to sort with. In this case the lower
numbers represent older data so none of them need to be prefixed with
``^``.

Finally, the last configuration is a mix of the prior two:

.. code-block:: ini

    [accesslogs]
    type = "LogstreamerInput"
    file_match = '/var/log/nginx/(?P<Year>\d+)/(?P<Month>\d+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "Month", "^Seq"]

Hopefully this is all fairly straight forward by now.

Multiple Sequential (Rotating) Logfiles
---------------------------------------

Same as before, except now we need to differentiate the sequential
streams. We're only introducing a single parameter here that we've seen
before to handle the differentiation. Lets take the last case from
above and consider it a multiple sequential source.

Example directory layout:

.. code-block::

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
    file_match = '/var/log/nginx/(?P<DomainName>[^/]+/(?P<Year>\d+)/(?P<Month>\d+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "Month", "^Seq"]
    differentiator = ["nginx-", "DomainName", "-access"]

As in the case for a non-sequential logfile, we supply a differentiator
that will be used to file each sequential set of logfiles into a
separate logstream.


Custom String Mappings
======================

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

.. code-block::

    MonthName - English full month name or 3-letter version to the appropriate
                integer.
    DayName   - English full day name or 3-letter version to the appropriate
                integer.

If the last example above looked like this:

.. code-block::

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
    file_match = '/var/log/nginx/(?P<DomainName>[^/]+/(?P<Year>\d+)/(?P<MonthName>\s+)/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "MonthName", "^Seq"]
    differentiator = ["nginx-", "DomainName", "-access"]

LogstreamerInput will translate the 3-letter month names automatically
before sorting (If used in the differentiator, you will still get the
original matched string).

Custom Mappings
---------------

What if your logfiles (for reasons we won't speculate about) happened
to use Pharsi month names but Spanish day names such that it looked
like this?

.. code-block::

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
    file_match = '/var/log/nginx/(?P<Year>\d+)/(?P<Month>\s+)/(?P<Day>[^/]+/access\.log\.?(?P<Seq>\d*)'
    priority = ["Year", "Month", "Day", "^Seq"]

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

    The matched values are lower-cased before being looked up in the
    translation mappings, so you should always use lower-case keys for
    the translation map keys as above.

We left off the rest of the month names and day names not used for
example purposes. Note that if you prefer the week to begin on a
Saturday instead of Monday you can configure it that way.

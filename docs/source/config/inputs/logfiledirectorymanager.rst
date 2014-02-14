
LogfileDirectoryManagerInput
============================

.. deprecated:: 0.5
    This input has been superseded by :ref:`config_logstreamer_input`.

Scans for log files in a globbed directory path and when a new file matching
the specified path is discovered it will start an instance of the LogfileInput
plugin to process it. Each LogfileInput will inherit its configuration from
the manager's settings with the logfile property properly adjusted.

Parameters: (identical to LogfileInput with the following exceptions)

- logfile (string):
    A path with a globbed directory component pointing to a common (statically
    named) log file. Note that only directories can be globbed; the file itself
    must have the same name in each directory.
- seek_journal_name (string):
    With a LogfileInput it is possible to specify a particular name for the
    seek journal file that will be used. This is not possible with the
    LogfileDirectoryManagerInput; the seek_journal_name will always be auto-
    generated, and any attempt to specify a hard coded seek_journal_name will
    be treated as a configuration error.
- ticker_interval (uint):
    Time interval (in seconds) between directory scans for new log files.
    Defaults to 0 (only scans once on startup).

.. code-block:: ini

    [vhosts]
    type = "LogfileDirectoryManagerInput"
    logfile = "/var/log/vhost/*/apache.log"

.. note::

    The spawned LogfileInput plugins are named `manager_name`-`logfile` i.e.,

    - vhosts-/var/log/www/apache.log
    - vhosts-/var/log/internal/apache.log

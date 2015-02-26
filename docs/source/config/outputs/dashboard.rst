.. _config_dashboard_output:

Dashboard Output
================

Plugin Name: **DashboardOutput**

Specialized output plugin that listens for certain Heka reporting message
types and generates JSON data which is made available via HTTP for use in web
based dashboards and health reports.

Config:

- ticker_interval (uint):
    Specifies how often, in seconds, the dashboard files should be updated.
    Defaults to 5.
- message_matcher (string):
    Defaults to `"Type == 'heka.all-report' || Type == 'heka.sandbox-output'
    || Type == 'heka.sandbox-terminated'"`. Not recommended to change this
    unless you know what you're doing.
- address (string):
    An IP address:port on which we will serve output via HTTP. Defaults to
    "0.0.0.0:4352".
- working_directory (string):
    File system directory into which the plugin will write data files and from
    which it will serve HTTP. The Heka process must have read / write access
    to this directory. Relative paths will be evaluated relative to the Heka
    base directory. Defaults to `$(BASE_DIR)/dashboard`.
- static_directory (string):
    File system directory where the Heka dashboard source code can be found.
    The Heka process must have read access to this directory. Relative paths
    will be evaluated relative to the Heka base directory. Defaults to
    `${SHARE_DIR}/dasher`.

.. versionadded:: 0.7

- headers (subsection, optional):
    It is possible to inject arbitrary HTTP headers into each outgoing response
    by adding a TOML subsection entitled "headers" to you HttpOutput config
    section. All entries in the subsection must be a list of string values.


Example:

.. code-block:: ini

    [DashboardOutput]
    ticker_interval = 30

.. _config_process_directory_input:

Process Directory Input
=======================

Plugin Name: **ProcessDirectoryInput**

.. versionadded:: 0.5

The ProcessDirectoryInput periodically scans a filesystem directory looking
for ProcessInput configuration files. The ProcessDirectoryInput will maintain
a pool of running ProcessInputs based on the contents of this directory,
refreshing the set of running inputs as needed with every rescan. This allows
Heka administrators to manage a set of data collection processes for a running
hekad server without restarting the server.

Each ProcessDirectoryInput has a `process_dir` configuration setting, which is
the root folder of the tree where scheduled jobs are defined. It should
contain exactly one nested level of subfolders, named with ASCII numeric
characters indicating the interval, in seconds, between each process run.
These numeric folders must contain TOML files which specify the details
regarding which processes to run.

For example, a process_dir might look like this::


  -/usr/share/heka/processes/
   |-5
     |- check_myserver_running.toml
   |-61
     |- cat_proc_mounts.toml
     |- get_running_processes.toml
   |-302
     |- some_custom_query.toml

This indicates one process to be run every five seconds, two processes to be
run every 61 seconds, and one process to be run every 302 seconds.

Note that ProcessDirectoryInput will ignore any files that are not nested one
level deep, are not in a folder named for an integer 0 or greater, and do not
end with '.toml'. Each file which meets these criteria, such as those shown in
the example above, should contain the TOML configuration for exactly one
:ref:`config_process_input`, matching that of a standalone ProcessInput with
the following restrictions:

- The section name *must* be `ProcessInput`. Any TOML sections named anything
  other than ProcessInput will be ignored.

- Any specified `ticker_interval` value will be *ignored*. The ticker interval
  value to use will be parsed from the directory path.

By default, if the specified process fails to run or the ProcessInput config
fails for any other reason, ProcessDirectoryInput will log an error message and
continue, as if the ProcessInput's `can_exit` flag has been set to true.
If the managed ProcessInput's `can_exit` flag is manually set to `false`, it
will trigger a Heka shutdown.

Config:

- ticker_interval (int, optional):
    Amount of time, in seconds, between scans of the process_dir. Defaults to
    300 (i.e. 5 minutes).
- process_dir (string, optional):
    This is the root folder of the tree where the scheduled jobs are defined.
    Absolute paths will be honored, relative paths will be computed relative to
    Heka's globally specified share_dir. Defaults to "processes" (i.e.
    "$share_dir/processes").
- retries (RetryOptions, optional):
    A sub-section that specifies the settings to be used for restart behavior
    of the ProcessDirectoryInput (not the individual ProcessInputs, which are
    configured independently).
    See :ref:`configuring_restarting`

Example:

.. code-block:: ini

	[ProcessDirectoryInput]
	process_dir = "/etc/hekad/processes.d"
	ticker_interval = 120

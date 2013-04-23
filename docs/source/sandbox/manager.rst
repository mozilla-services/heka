.. _sandboxmanager:

===============
Sandbox Manager
===============
The SandboxManagerFilter allows SandboxFilters to be dynamically started and
stopped using a signed Heka message.  The intent is to have one 
manager per access control group each with their own message signing key. Users
in each group can submit a signed control message to manage any filters running
under the associated manager.  A signed message is not an enforced requirement
but it is highly recommended in order to restrict access to this functionality.

Filter Parameters
=================
:ref:`common_filter_parameters`

.. _sandboxmanagerfilter_settings: 

SandboxManagerFilter Settings
=============================
- working_directory (string): The directory where the filter configurations, code, and states are preserved.  The directory can be unique or shared between sandbox managers since the filter names are unique per manager.
- max_filters (uint): The maximum number of filters this manager can run.

Example

.. code-block:: ini

    [OpsSandboxManager]
    type = "SandboxManagerFilter"
    message_signer = "ops"
    message_matcher = "Type == 'heka.control.sandbox'"
    working_directory = "/var/heka/sandbox"
    max_filters = 100

Control Message
===============
The sandbox manager control message is a regular Heka message with the following
variables set to the specified values. 

Starting a SandboxFilter
------------------------
- Type: "heka.control.sandbox"
- Payload: *sandbox code*
- Fields[action]: "load"
- Fields[config]: the TOML configuration for the SandboxFilter :ref:`sandboxfilter_settings`

Stopping a SandboxFilter
------------------------
- Type: "heka.control.sandbox"
- Fields[action]: "unload"
- Fields[name]: The SandboxFilter name specified in the configuration


sbmgr
=====
Sbmgr is a tool for managing (starting/stopping) sandbox filters by generating
the control messages defined above.

Command Line Options
--------------------
sbmgr [``-config`` `config_file`] [``-action`` `load|unload`] [``-filtername`` `specified on unload`]
[``-script`` `sandbox script filename`] [``-scriptconfig`` `sandbox script configuration filename`]

sbmgrload
=========
Sbmgrload is a test tool for starting/stopping a large number of sandboxes.  The
script and configuration are built into the tool and the filters will be named:
CounterSandbox\ **N** where **N** is the instance number.

Command Line Options
--------------------
sbmgrload [``-config`` `config_file`] [``-action`` `load|unload`] [``-num`` `number of sandbox instances`]


Configuration Variables
-----------------------
- ip_address (string): IP address of the Heka server.
- signer (object): Signer information for the encoder.
    - name (string): The name of the signer.
    - hmac_hash (string): md5 or sha1
    - hmac_key (string): The key the message will be signed with.
    - version (int): The version number of the hmac_key. 

Example

.. code-block:: ini

    ip_address          = "127.0.0.1:5565"
    [signer]
        name         = "test"
        hmac_hash    = "md5"
        hmac_key     = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
        version      = 0


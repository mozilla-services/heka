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

    [OpsSandboxManager.settings]
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



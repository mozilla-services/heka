.. _sandboxmanager:

.. include:: ../config/filters/sandboxmanager.rst
   :start-line: 1

Control Message
---------------
The sandbox manager control message is a regular Heka message with the following
variables set to the specified values.

Starting a SandboxFilter

- Type: "heka.control.sandbox"
- Payload: *sandbox code*
- Fields[action]: "load"
- Fields[config]: the TOML configuration for the :ref:`config_sandbox_filter`

Stopping a SandboxFilter

- Type: "heka.control.sandbox"
- Fields[action]: "unload"
- Fields[name]: The SandboxFilter name specified in the configuration


heka-sbmgr
----------
Heka Sbmgr is a tool for managing (starting/stopping) sandbox filters by generating
the control messages defined above.

Command Line Options

heka-sbmgr [``-config`` `config_file`] [``-action`` `load|unload`] [``-filtername`` `specified on unload`]
[``-script`` `sandbox script filename`] [``-scriptconfig`` `sandbox script configuration filename`]

Configuration Variables

- ip_address (string): IP address of the Heka server.
- use_tls (bool): Specifies whether or not SSL/TLS encryption should be used for the TCP connections. Defaults to false.
- signer (object): Signer information for the encoder.
    - name (string): The name of the signer.
    - hmac_hash (string): md5 or sha1
    - hmac_key (string): The key the message will be signed with.
    - version (int): The version number of the hmac_key.
- tls (TlsConfig): A sub-section that specifies the settings to be used for any SSL/TLS encryption. This will only have any impact if `use_tls` is set to true. See :ref:`tls`.

Example

.. code-block:: ini

    ip_address       = "127.0.0.1:5565"
    use_tls          = true
    [signer]
        name         = "test"
        hmac_hash    = "md5"
        hmac_key     = "4865ey9urgkidls xtb0[7lf9rzcivthkm"
        version      = 0

    [tls]
        cert_file = "heka.crt"
        key_file = "heka.key"
        client_auth = "NoClientCert"
        prefer_server_ciphers = true
        min_version = "TLS11"

heka-sbmgrload
--------------
Heka Sbmgrload is a test tool for starting/stopping a large number of
sandboxes. The script and configuration are built into the tool and the filters
will be named: CounterSandbox\ **N** where **N** is the instance number.

Command Line Options

heka-sbmgrload [``-config`` `config_file`] [``-action`` `load|unload`] [``-num`` `number of sandbox instances`]

Configuration Variables (same as heka-sbmgr)

.. _sandbox_manager_tutorial:

Tutorial - How to use the dynamic sandboxes
===========================================

SandboxManager/Heka Setup
-------------------------

1. Configure the SandboxManagerFilter.

The SandboxManagerFilters are defined in the hekad configuration file and are
created when hekad starts. The manager provides a location/namespace for
SandboxFilters to run and controls access to this space via a signed Heka
message. By associating a message_signer with the manager we can restrict who
can load and unload the associated filters. Lets start by  configuring a
SandboxManager for a specific set of users; platform developers. Choose a
unique filter name [PlatformDevs] and a signer name "PlatformDevs", in this
case we will use the same name for each.

.. code-block:: ini

    [PlatformDevs]
    type = "SandboxManagerFilter"
    message_signer = "PlatformDevs"
    working_directory = "/var/heka/sandbox"
    max_filters = 100


2. Configure the input that will receive the SandboxManager control messages.

For this setup we will extend the current TCP input to handle our signed
messages. The signer section consists of the signer name followed by an
underscore and the key version number (the reason for this notation is to
simply flatten the signer configuration structure into a single map).
Multiple key versions are allowed to be active at the same time facilitating
the rollout of new keys.

.. code-block:: ini

    [TCP:5565]
    type = "TcpInput"
    address = ":5565"
        [TCP:5565.signer.PlatformDevs_0]
        hmac_key = "Old Platform devs signing key"
        [TCP:5565.signer.PlatformDevs_1]
        hmac_key = "Platform devs signing key"

3. Configure the sandbox manager utility (sbmgr). The signer information must
exactly match the values in the input configuration above otherwise the
messages will be discarded. Save the file as PlatformDevs.toml.

.. code-block:: ini

    ip_address       = ":5565"
    [signer]
        name         = "PlatformDevs"
        hmac_hash    = "md5"
        hmac_key     = "Platform devs signing key"
        version      = 1

SandboxFilter Setup
-------------------

1. Create a SandboxFilter script and save it as "example.lua". See
   :ref:`lua_tutorials` for more detail.

.. code-block:: lua

    require "circular_buffer"

    data = circular_buffer.new(1440, 1, 60) -- message count per minute
    local COUNT = data:set_header(1, "Messages", "count")
    function process_message ()
        local ts = read_message("Timestamp")
        data:add(ts, COUNT, 1)
        return 0
    end

    function timer_event(ns)
        inject_payload("cbuf", "", data)
    end

2. Create the SandboxFilter configuration and save it as "example.toml".

The only difference between a static and dynamic SandboxFilter configuration is
the filename. In the dynamic configuration it can be left blank or left out
entirely. The manager will assign the filter a unique system wide name, in this
case, "PlatformDevs-Example".

.. code-block:: ini

    [Example]
    type = "SandboxFilter"
    message_matcher = "Type == 'Widget'"
    ticker_interval = 60
    filename = ""
    preserve_data = false

3. Load the filter using sbmgr.

::

    sbmgr -action=load -config=PlatformDevs.toml -script=example.lua -scriptconfig=example.toml

If you are running the :ref:`config_dashboard_output` the following links are
available:

- Information about the running filters:
  http://localhost:4352/heka_report.html.

- Graphical Output (after 1 minute in this case): http://localhost:4352
  /PlatformDevs-Example.html

Otherwise

- Information about the terminated filters: http://localhost:4352/heka_sandbox_termination.html.

.. note::

    A running filter cannot be 'reloaded' it must be unloaded and loaded again.
    During the unload/load process some data can be missed and gaps will be
    created. In the future we hope to remedy this but for now it is a
    limitation of the dynamic sandbox.

4. Unload the filter using sbmgr.

::

    sbmgr -action=unload -config=PlatformDevs.toml -filtername=Example

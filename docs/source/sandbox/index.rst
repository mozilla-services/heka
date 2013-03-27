.. _sandbox:

============
Heka Sandbox
============

Sandboxes provide a dynamic and isolated execution environment for data 
analysis. They allow real time access to data in production without 
jeopardizing the integrity or performance of the monitoring infrastructure.
This broadens the audience that the data can be exposed to and facilitates 
new uses of the data (i.e. debugging, monitoring, dynamic provisioning, 
SLA analysis, intrusion detection, ad-hoc reporting, etc.)

Features
--------
- dynamic loading - ability to start/stop on a self-service basis
- small - memory requirements are about 16 KiB for a basic sandbox
- fast - microsecond execution times
- stateful - ability to resume where it left off after a restart/reboot
- isolated - failures are contained and malfunctioning sandboxes are terminated

Sandbox Configuration
----------------------------
Heka Sandbox Filter Configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
name
  The name of this specific instance of the sandbox

type
  The Heka filter type - must be 'SandboxFilter'

message_matcher
  The match string used by the router to determine if this sandbox should
  receive the message 

  see: :ref:`message_matcher`

ticker_interval
  The number of seconds after which the timer_event callback will be executed

Sandbox Specific Configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
type
  The language the sandbox is written in.  Currently the only valid option is
  'lua'

filename
  For a static configuration this is the full path to the sandbox code.
  For a dynamic configuration only the filename component of the path is 
  used (the physical location on disk is controlled by the SandboxManager 
  filter)

preserve_data
  Boolean indicating if the sandbox global data should be preserved/restored on
  shutdown/startup. The preserved data is stored along side the sandbox code 
  i.e. counter.lua.data so Heka must have read/write permissions to that 
  directory.

memory_limit
  Number of bytes the sandbox is allowed to consume before being terminated 
  (max 8MiB)

instruction_limit
  Number of instructions the sandbox is allowed the execute before being 
  terminated (max 1M)

output_limit
  The largest payload the sandbox is allowed to output before being terminated
  (max 63KiB)

::

    {
        "name": "hekabench_counter",
        "type": "SandboxFilter",
        "message_matcher": "Type == 'hekabench' && EnvVersion == '0.8'",
        "ticker_interval" : 1,
            "sandbox": {
                "type" : "lua",
                "filename" : "counter.lua",
                "preserve_data" : true,
                "memory_limit" : 32767,
                "instruction_limit" : 1000,
                "output_limit" : 1024
            }
    }

Available Sandbox Types
-----------------------
.. toctree::
   lua
   :maxdepth: 3

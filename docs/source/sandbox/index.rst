.. _sandbox:

=======
Sandbox
=======

Sandboxes are Heka plugins that are implemented in a sandboxed scripting 
language. They provide a dynamic and isolated execution environment
for data parsing, transformation, and analysis.  They allow real time access to
data in production without jeopardizing the integrity or performance of the 
monitoring infrastructure and do not require Heka to be recompiled.
This broadens the audience that the data can be exposed to and facilitates new
uses of the data (i.e. debugging, monitoring, dynamic provisioning,  SLA
analysis, intrusion detection, ad-hoc reporting, etc.)

Features
========

- dynamic loading
    - SandboxFilters can be started/stopped on a self-service basis while Heka is running
    - SandboxDecoder can only be started/stopped on a Heka restart but no recompilation is required to add new functionality.
- small - memory requirements are about 16 KiB for a basic sandbox
- fast - microsecond execution times
- stateful - ability to resume where it left off after a restart/reboot
- isolated - failures are contained and malfunctioning sandboxes are terminated

.. include:: lua.rst
.. include:: input.rst
.. include:: decoder.rst
.. include:: filter.rst
.. include:: encoder.rst
.. include:: output.rst
.. include:: module.rst
.. include:: lpeg.rst
.. include:: manager.rst
.. include:: development.rst
.. include:: cookbook.rst

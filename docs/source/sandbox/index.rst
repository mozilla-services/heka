.. _sandbox:

=================
Sandboxed Filters
=================

Sandboxed filters are Heka filter plugins that are implemented in a sandboxed
scripting language. They provide a dynamic and isolated execution environment
for data analysis, and allow real time access to data in production without
jeopardizing the integrity or performance of the monitoring infrastructure.
This broadens the audience that the data can be exposed to and facilitates new
uses of the data (i.e. debugging, monitoring, dynamic provisioning,  SLA
analysis, intrusion detection, ad-hoc reporting, etc.)

Features
========

- dynamic loading - ability to start/stop on a self-service basis
- small - memory requirements are about 16 KiB for a basic sandbox
- fast - microsecond execution times
- stateful - ability to resume where it left off after a restart/reboot
- isolated - failures are contained and malfunctioning sandboxes are terminated

Sandbox Manager
===============
:ref:`sandboxmanager`

Sandbox Filter
==============
:ref:`sandboxfilter`

Sandbox Types
=============

.. toctree::
   lua
   :maxdepth: 2


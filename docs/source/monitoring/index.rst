.. _internal_monitoring:

=========================
Monitoring Internal State
=========================

Heka Reports
------------

Heka can emit metrics about it's internal state to either an outgoing Heka
message (and, through the DashboardOutput, to a web dashboard) or to stdout.
Sending SIGUSR1 to hekad on a UNIX will send a plain text report to stdout. On
Windows, you will need to send signal 10 to the hekad process using Powershell.

Sample text output ::

    ========[heka.all-report]========
    inputRecycleChan:
        InChanCapacity: 100
        InChanLength: 99
    injectRecycleChan:
        InChanCapacity: 100
        InChanLength: 98
    Router:
        InChanCapacity: 50
        InChanLength: 0
        ProcessMessageCount: 26
    ProtobufDecoder-0:
        InChanCapacity: 50
        InChanLength: 0
    ProtobufDecoder-1:
        InChanCapacity: 50
        InChanLength: 0
    ProtobufDecoder-2:
        InChanCapacity: 50
        InChanLength: 0
    ProtobufDecoder-3:
        InChanCapacity: 50
        InChanLength: 0
    DecoderPool-ProtobufDecoder:
        InChanCapacity: 4
        InChanLength: 4
    OpsSandboxManager:
        InChanCapacity: 50
        InChanLength: 0
        MatchChanCapacity: 50
        MatchChanLength: 0
        MatchAvgDuration: 0
        ProcessMessageCount: 0
    hekabench_counter:
        InChanCapacity: 50
        InChanLength: 0
        MatchChanCapacity: 50
        MatchChanLength: 0
        MatchAvgDuration: 445
        ProcessMessageCount: 0
        InjectMessageCount: 0
        Memory: 20644
        MaxMemory: 20644
        MaxInstructions: 18
        MaxOutput: 0
        ProcessMessageAvgDuration: 0
        TimerEventAvgDuration: 78532
    LogOutput:
        InChanCapacity: 50
        InChanLength: 0
        MatchChanCapacity: 50
        MatchChanLength: 0
        MatchAvgDuration: 406
    DashboardOutput:
        InChanCapacity: 50
        InChanLength: 0
        MatchChanCapacity: 50
        MatchChanLength: 0
        MatchAvgDuration: 336
    ========

To enable the HTTP interface, you will need to enable the dashboard output
plugin, see :ref:`config_dashboard_output`.

Aborting When Wedged
--------------------

In some cases a misconfigured Heka instance or some other conditions can cause
Heka to suffer from "pack exhaustion", resulting in the process becoming wedged
such that no more data flows through the pipeline. In this case Heka will not
shut down cleanly when sent a SIGINT signal via ``ctrl-c``.

When this happens, Heka administrators should send a SIGUSR2 (or signal 11 on
Windows) to the hekad process. If Heka detects that messages are successfully
flowing through the system this will have no impact, but if Heka can verify
that the pipeline is wedged then the following sequence of events will be
triggered:

* A Heka report will be generated and output to the console, hopefully
  providing insight into what caused the wedging.

* An `abort` signal will propagate through the Heka pipeline, causing any
  wedged sandboxes to become unwedged and immediately exit, preserving their
  state if specified with the ``preserve_data = true`` config setting.

* All remaining sandboxes will be shut down, preserving state if specified.

* Heka will send itself a SIGQUIT signal, causing the process to immediately
  terminate, dumping the stack traces for all running goroutines to the
  console for forensic purposes.

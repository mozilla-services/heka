.. _internal_monitoring:

=========================
Monitoring Internal State
=========================

Heka can emit metrics about it's internal state to either an outgoing
Heka message (and, through the DashboardOutput, to a web dashboard) or
to stdout.
Sending SIGUSR1 to hekad on a UNIX will send a plain text report
tostdout. On Windows, you will need to send signal
10 to the hekad process using Powershell.

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
    JsonDecoder-0:
        InChanCapacity: 50
        InChanLength: 0
    JsonDecoder-1:
        InChanCapacity: 50
        InChanLength: 0
    JsonDecoder-2:
        InChanCapacity: 50
        InChanLength: 0
    JsonDecoder-3:
        InChanCapacity: 50
        InChanLength: 0
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
    DecoderPool-JsonDecoder:
        InChanCapacity: 4
        InChanLength: 4
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

To enable the HTTP interface, you will need to enable the
dashboard output plugin, see :ref:`config_dashboard_output`.

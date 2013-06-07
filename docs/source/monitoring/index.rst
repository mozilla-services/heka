.. _internal_monitoring:

=========================
Monitoring Internal State
=========================

Heka can emit metrics about it's internal state to either the web or
to stdout.  Sending SIGUSR1 to hekad on a UNIX will send a pipe
delimited format to stdout. On Windows, you will need to send signal
10 to the hekad process using Powershell.

Sample pipe delimited report output ::

    ========[heka.all-report]========
    InChanCapacity|InChanLength|MatchChanCapacity|MatchChanLength|MatchAvgDuration|ProcessMessageCount|InjectMessageCount|Memory|MaxMemory|MaxInstructions|MaxOutput|ProcessMessageAvgDuration|TimerEventAvgDuration
    100|99|||||||||||
    100|98|||||||||||
    50|0||||26|||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    50|0|||||||||||
    4|4|||||||||||
    4|4|||||||||||
    50|0|50|0|0|0|||||||
    50|0|50|0|445|0|0|20644|20644|18|0|0|78532
    50|0|50|0|406||||||||
    50|0|50|0|336||||||||
    ========

To enable the HTTP interface, you will need to enable the
dashboard output plugin, see :ref:`config_dashboard_output`.

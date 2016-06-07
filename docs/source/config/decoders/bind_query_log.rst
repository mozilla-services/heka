.. _config_bind_query_log_decoder:

BIND Query Log Decoder
========================

.. versionadded:: 0.11

| Plugin Name: **SandboxDecoder**
| File Name: **lua_decoders/bind_query_log.lua**

Parses DNS query logs from the BIND DNS server.

**Note**: You must have the `print-time`, `print-severity` and `print-category` options all set to **yes** in the logging configuration section of your `named.conf` file:

.. code-block:: bash

    channel query_log {
      file "/var/log/named/named_query.log" versions 3 size 5m;
      severity info;
      print-time yes;
      print-severity yes;
      print-category yes;
    };

Config:

- type (string, optional, default nil):
    Sets the message 'Type' header to the specified value

*Example Heka Configuration*

.. code-block:: ini

    [BindQueryLogInput]
    type = "LogstreamerInput"
    decoder = "BindQueryLogDecoder"
    file_match = 'named_query.log'
    log_directory = "/var/log/named"

    [BindQueryLogDecoder]
    type = "SandboxDecoder"
    filename = "lua_decoders/bind_query_log.lua"
      [BindQueryLogDecoder.config]
      type = "bind.query"

*Example Heka Message*

2016/04/25 17:31:37 
:Timestamp: 2016-04-26 00:31:37 +0000 UTC
:Type: bind_query
:Hostname: ns1.company.com
:Pid: 0
:Uuid: 09a83ad2-89c0-4a7d-adfc-0e225e1c1ad6
:Logger: bind_query_log_input
:Payload: 27-May-2015 21:06:49.246 queries: info: client 10.0.1.70#41242 (webserver.company.com): query: webserver.company.com IN A +E (10.0.1.71)
:EnvVersion: 
:Severity: 7
:Fields:
    | name:"QueryFlags" type:string value:["recursion requested","EDNS used"]
    | name:"ClientIP" type:string value:"10.0.1.70" representation:"ipv4"
    | name:"ServerRespondingIP" type:string value:"10.0.1.71" representation:"ipv4"
    | name:"RecordType" type:string value:"A"
    | name:"QueryName" type:string value:"webserver"
    | name:"RecordClass" type:string value:"IN"
    | name:"Timestamp" type:double value:1.432760809e+18
    | name:"QueryDomain" type:string value:"company.com"
    | name:"FullQuery" type:string value:"webserver.company.com"

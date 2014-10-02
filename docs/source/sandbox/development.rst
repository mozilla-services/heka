.. _sandbox_development:

Sandbox Development
===================

Decoders
--------
Since decoders cannot be dynamically loaded and they stop Heka processing on
fatal errors they must be developed outside of a production enviroment. Most
Lua  decoders are LPeg based as it is the best way to parse and transform data
within the sandbox. The other alternatives are the built-in Lua pattern
matcher or the  JSON parser with a manual transformation.

1. Procure some sample data to be used as test input.

    .. code-block:: bash

        timestamp=time_t key1=data1 key2=data2

2. Configure a simple LogstreamerInput to deliver the data to your decoder.

    .. code-block:: ini

        [LogstreamerInput]
        log_directory = "."
        file_match = 'data\.log'
        decoder = "SandboxDecoder"

3. Configure your test decoder.

    .. code-block:: ini

        [SandboxDecoder]
        filename = "decoder.lua"

4. Configure the DasboardOutput for visibility into the decoder (performance,
   memory usage, messages processed/failed, etc.)

    .. code-block:: ini

        [DashboardOutput]
        address = "127.0.0.1:4352"
        ticker_interval = 10
        working_directory = "dashboard"
        static_directory = "/usr/share/heka/dasher"

5. Configure a LogOutput to display the generated messages.

    .. code-block:: ini

        [LogOutput]
        message_matcher = "TRUE"

6. Build the decoder.
    The decoder will receive a message from an input plugin. The input may
    have set some additional message headers but the 'Payload' header contains
    the data for the decoder. The decoder can access the payload using
    read_message("Payload").  The payload can be used to construct an entirely
    new message, multiple messages or modify any part of the existing message
    (see inject_message, write_message in the :ref:`lua` API).  Message
    headers not modified by the decoder are left intact and in the case of
    multiple message injections the initial message header values are
    duplicated for each message.

    #. LPeg grammar.
        Incrementally build and test your grammar using http://lpeg.trink.com.

    #. Lua pattern matcher.
        Test match expressions using http://www.lua.org/cgi-bin/demo.

    #. JSON parser.
        For data transformation use the LPeg/Lua matcher links above.
        Something like simple field remapping i.e. `msg.Hostname = json.host`
        can be verified in the LogOutput.

7. Run Heka with the test configuration.

8. Inspect/verify the messages written by LogOutput.
    

Filters
-------
Since filters can be dynamically loaded it is recommended you develop them in
production with live data.

1. Read :ref:`sandbox_manager_tutorial`

**OR**

1. If you are developing the filter in conjunction with the decoder you can
   add it to the test configuration.

    .. code-block:: ini

        [SandboxFilter]
        filename = "filter.lua"

2. Debugging

    1. Watch for a dashboard sandbox termination report. The termination
       message provides the line number and cause of the failure. These are
       usually straight forward to correct and commonly caused by a syntax
       error in the script or invalid assumptions about the data (e.g. `cnt =
       cnt + read_message("Fields[counter]")` will fail if the counter field
       doesn't exist or is non-numeric due to a error in the data).

    2. No termination report and the output does not match expectations. These
       are usually a little harder to debug.

        1. Check the Heka dasboard to make sure the router is sending messages
           to the plugin. If not, verify your message_matcher configuration.

        2. Visually review the the plugin for errors. Are the message field
           names correct, was the result of the cjson.decode tested, are the
           output variables actually being assigned to and output/injected,
           etc.

        3. Add a debug output message with the pertinent information.
 
        .. code-block:: lua

            require "string"
            require "table"
            local dbg = {}

            -- table.insert(dbg, string.format("Entering function x arg1: %s", arg1))
            -- table.insert(dbg, "Exiting function x")

            inject_payload("txt", "debug", table.concat(dbg, "\n"))

        4. LAST RESORT: Move the filter out of production, turn on
           preservation, run the tests, stop Heka, and review the entire
           preserved state of the filter.

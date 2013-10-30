# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

# We use one spelling of the mockgen command for mocks of interfaces in our own
# packages...

set(MOCKGEN_EXECUTABLE "${PROJECT_PATH}/bin/mockgen${CMAKE_EXECUTABLE_SUFFIX}")
macro(add_internal_mock package destination mocked_object source)
    set(_path "${HEKA_PATH}/${package}/${destination}")
    set(_MOCK_LIST ${_MOCK_LIST} ${_path})
    add_custom_command(OUTPUT ${_path}
    COMMAND ${MOCKGEN_EXECUTABLE}
    -package=${package}
    -destination="${_path}"
    -self_package=github.com/mozilla-services/heka/${package}
    github.com/mozilla-services/heka/${package}
    ${mocked_object}
    DEPENDS "${CMAKE_SOURCE_DIR}/${package}/${source}"
    WORKING_DIRECTORY ${HEKA_PATH}
    COMMENT "Built ${destination}")
endmacro(add_internal_mock)

macro(add_external_mock package destination mocked_package mocked_object)
    set(_path "${HEKA_PATH}/${package}/${destination}")
    set(_MOCK_LIST ${_MOCK_LIST} ${_path})
    add_custom_command(OUTPUT ${_path}
    COMMAND ${MOCKGEN_EXECUTABLE}
    -package=${package}
    -destination="${package}/${destination}"
    ${mocked_package}
    ${mocked_object}
    WORKING_DIRECTORY ${HEKA_PATH}
    COMMENT "Built ${destination}")
endmacro(add_external_mock)

add_internal_mock(pipeline mock_pluginhelper_test.go        PluginHelper        config.go)
add_internal_mock(pipeline mock_pluginrunner_test.go        PluginRunner        pipeline_runner.go)
add_internal_mock(pipeline mock_decoder_test.go             Decoder             decoders.go)
add_internal_mock(pipeline mock_decoderset_test.go          DecoderSet          decoders.go)
add_internal_mock(pipeline mock_decoderrunner_test.go       DecoderRunner       decoders.go)
add_internal_mock(pipeline mock_inputrunner_test.go         InputRunner         inputs.go)
add_internal_mock(pipeline mock_filterrunner_test.go        FilterRunner        filters.go)
add_internal_mock(pipeline mock_outputrunner_test.go        OutputRunner        outputs.go)
add_internal_mock(pipeline mock_input_test.go               Input               inputs.go)
add_internal_mock(pipeline mock_whisperrunner_test.go       WhisperRunner       whisper.go)
add_internal_mock(pipeline mock_amqpconnection_test.go      AMQPConnection      amqp_plugin.go)
add_internal_mock(pipeline mock_amqpchannel_test.go         AMQPChannel         amqp_plugin.go)
add_internal_mock(pipeline mock_amqpconnectionhub_test.go   AMQPConnectionHub   amqp_plugin.go)
add_internal_mock(pipeline mock_stataccumulator_test.go     StatAccumulator     stat_accum_input.go)

add_external_mock(testsupport mock_amqp_acknowledger.go github.com/streadway/amqp   Acknowledger)
add_external_mock(testsupport mock_net_conn.go          net                         Conn)
add_external_mock(testsupport mock_net_listener.go      net                         Listener)
add_external_mock(testsupport mock_net_error.go         net                         Error)

add_custom_target(mocks ALL DEPENDS gomock lua-5_1_5 ${_MOCK_LIST})


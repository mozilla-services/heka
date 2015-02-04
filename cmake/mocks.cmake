# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

# We use one spelling of the mockgen command for mocks of interfaces in our own
# packages...

set(MOCKGEN_EXECUTABLE "${PROJECT_PATH}/bin/mockgen${CMAKE_EXECUTABLE_SUFFIX}")
macro(add_internal_mock package destination mocked_object source)
    set(_path "${HEKA_PATH}/${package}/${destination}")
    set(_MOCK_LIST ${_MOCK_LIST} ${_path})
    get_filename_component(_package_short ${package} NAME)
    add_custom_command(OUTPUT ${_path}
    COMMAND ${MOCKGEN_EXECUTABLE}
    -package=${_package_short}
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
    get_filename_component(_package_short ${package} NAME)
    add_custom_command(OUTPUT ${_path}
    COMMAND ${CMAKE_COMMAND} -E make_directory "${HEKA_PATH}/${package}"
    COMMAND ${MOCKGEN_EXECUTABLE}
    -package=${_package_short}
    -destination="${package}/${destination}"
    ${mocked_package}
    ${mocked_object}
    WORKING_DIRECTORY ${HEKA_PATH}
    COMMENT "Built ${destination}")
endmacro(add_external_mock)

#
# `pipeline` package mocks
#
add_internal_mock(pipeline mock_pluginhelper_test.go        PluginHelper        config.go)
add_internal_mock(pipeline mock_pluginrunner_test.go        PluginRunner        pipeline_runner.go)
add_internal_mock(pipeline mock_decoder_test.go             Decoder             plugin_interfaces.go)
add_internal_mock(pipeline mock_decoderrunner_test.go       DecoderRunner       plugin_runners.go)
add_internal_mock(pipeline mock_inputrunner_test.go         InputRunner         plugin_runners.go)
add_internal_mock(pipeline mock_filterrunner_test.go        FilterRunner        plugin_runners.go)
add_internal_mock(pipeline mock_outputrunner_test.go        OutputRunner        plugin_runners.go)
add_internal_mock(pipeline mock_input_test.go               Input               plugin_interfaces.go)
add_internal_mock(pipeline mock_stataccumulator_test.go     StatAccumulator     stat_accum_input.go)
add_internal_mock(pipeline mock_deliverer_test.go           Deliverer           plugin_runners.go)
add_internal_mock(pipeline mock_splitterrunner_test.go      SplitterRunner      splitter_runner.go)

add_external_mock(pipelinemock mock_pluginhelper.go github.com/mozilla-services/heka/pipeline   PluginHelper)
add_external_mock(pipelinemock mock_filterrunner.go github.com/mozilla-services/heka/pipeline   FilterRunner)
add_external_mock(pipelinemock mock_decoderrunner.go github.com/mozilla-services/heka/pipeline  DecoderRunner)
add_external_mock(pipelinemock mock_outputrunner.go github.com/mozilla-services/heka/pipeline   OutputRunner)
add_external_mock(pipelinemock mock_inputrunner.go github.com/mozilla-services/heka/pipeline    InputRunner)
add_external_mock(pipelinemock mock_decoder.go github.com/mozilla-services/heka/pipeline        Decoder)
add_external_mock(pipelinemock mock_stataccumulator.go github.com/mozilla-services/heka/pipeline StatAccumulator)
add_external_mock(pipelinemock mock_deliverer.go github.com/mozilla-services/heka/pipeline Deliverer)
add_external_mock(pipelinemock mock_splitterrunner.go github.com/mozilla-services/heka/pipeline SplitterRunner)

add_external_mock(pipeline/testsupport mock_net_conn.go          net                         Conn)
add_external_mock(pipeline/testsupport mock_net_listener.go      net                         Listener)
add_external_mock(pipeline/testsupport mock_net_error.go         net                         Error)

#
# `plugins` package and sub-package mocks
#
add_internal_mock(plugins/graphite mock_whisperrunner_test.go   WhisperRunner       whisper.go)

add_internal_mock(plugins/amqp mock_amqpconnection_test.go      AMQPConnection      types.go)
add_internal_mock(plugins/amqp mock_amqpchannel_test.go         AMQPChannel         types.go)
add_internal_mock(plugins/amqp mock_amqpconnectionhub_test.go   AMQPConnectionHub   types.go)

add_external_mock(plugins/testsupport mock_amqp_acknowledger.go github.com/streadway/amqp   Acknowledger)


add_custom_target(mocks ALL DEPENDS gomock ${SANDBOX_PACKAGE} ${_MOCK_LIST})


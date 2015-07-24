# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

cmake_minimum_required(VERSION 2.8.7 FATAL_ERROR)
project(heka C)
set(CPACK_PACKAGE_DESCRIPTION_SUMMARY "High performance data gathering, analysis, monitoring, and reporting.")
set(CPACK_PACKAGE_VERSION_MAJOR 0)
set(CPACK_PACKAGE_VERSION_MINOR 11)
set(CPACK_PACKAGE_VERSION_PATCH 0)
set(CPACK_PACKAGE_VENDOR "Mozilla")
set(CPACK_RESOURCE_FILE_LICENSE "${CMAKE_SOURCE_DIR}/LICENSE.txt")
set(CPACK_PACKAGE_CONTACT "heka@mozilla.org")
set(CPACK_PACKAGE_NAME ${CMAKE_PROJECT_NAME})

set(CMAKE_MODULE_PATH "${CMAKE_SOURCE_DIR}/cmake")

find_package(Go 1.4 REQUIRED)
find_package(Git REQUIRED)
find_package(Protobuf 2.3 QUIET)
set(CPACK_PACKAGE_FILE_NAME ${CMAKE_PROJECT_NAME}-${CPACK_PACKAGE_VERSION_MAJOR}_${CPACK_PACKAGE_VERSION_MINOR}_${CPACK_PACKAGE_VERSION_PATCH}-${GO_PLATFORM}-${GO_ARCH})
set(CMAKE_INSTALL_PREFIX ${CMAKE_PROJECT_NAME})
set(PROJECT_PATH "${CMAKE_BINARY_DIR}/heka")
set(HEKA_PATH "${PROJECT_PATH}/src/github.com/mozilla-services/heka")
set(LUA_INCLUDE_PATH "${PROJECT_PATH}/include")
set(LUA_LIB_PATH "${PROJECT_PATH}/lib")
set(HEKA_EXE "${PROJECT_PATH}/bin/hekad${CMAKE_EXECUTABLE_SUFFIX}")
set(FLOOD_EXE "${PROJECT_PATH}/bin/heka-flood${CMAKE_EXECUTABLE_SUFFIX}")
set(SBMGR_EXE "${PROJECT_PATH}/bin/heka-sbmgr${CMAKE_EXECUTABLE_SUFFIX}")
set(SBMGRLOAD_EXE "${PROJECT_PATH}/bin/heka-sbmgrload${CMAKE_EXECUTABLE_SUFFIX}")
set(INJECT_EXE "${PROJECT_PATH}/bin/heka-inject${CMAKE_EXECUTABLE_SUFFIX}")
set(LOGSTREAMER_EXE "${PROJECT_PATH}/bin/heka-logstreamer${CMAKE_EXECUTABLE_SUFFIX}")
set(HEKA_CAT_EXE "${PROJECT_PATH}/bin/heka-cat${CMAKE_EXECUTABLE_SUFFIX}")

option(INCLUDE_SANDBOX "Include Lua sandbox" on)
option(INCLUDE_MOZSVC "Include the Mozilla services plugins" on)
option(INCLUDE_DOCKER_PLUGINS "Include Docker plugins" on)

find_path(INCLUDE_GEOIP GeoIP.h /usr/local/include /usr/include /opt/local/include)
if (NOT INCLUDE_GEOIP)
    message(STATUS "GeoIP.h was not found, GeoIP functionality will not be included in this build.")
else()
    message(STATUS "GeoIP.h found. Enabling GeoIP plugin.")
    set(TAGS "${TAGS} geoip")
    set(PLUGIN_LOADER ${PLUGIN_LOADER} "github.com/mozilla-services/heka/plugins/geoip")
endif()

if (INCLUDE_DOCKER_PLUGINS)
    message(STATUS "Docker plugins enabled.")
    set(PLUGIN_LOADER ${PLUGIN_LOADER} "github.com/mozilla-services/heka/plugins/docker")
endif()

option(BENCHMARK "Enable the benchmark tests" off)
if (BENCHMARK)
    set(BENCHMARK_FLAG -bench .)
endif()

option(COVERAGE "Enable code coverage" off)
if (COVERAGE)
    set(COVERAGE_FLAG -coverprofile coverage.out)
endif()

set(INCLUDE_DOCUMENTATION true)
find_program(SPHINX_BUILD_EXECUTABLE sphinx-build PATH_SUFFIXES bin)
if (NOT SPHINX_BUILD_EXECUTABLE)
    message(STATUS "sphinx-build was not found, the documentation will not be generated.")
    set(INCLUDE_DOCUMENTATION false)
endif()

set(LUA_SHARED_LIBRARY_SUFFIX ${CMAKE_SHARED_LIBRARY_SUFFIX})
if (MINGW)
    set(ADDRESS_MODEL 64)
    if (GO_ARCH STREQUAL "386")
        set(ADDRESS_MODEL 32)
    endif()
    set(LDFLAGS "-ldflags=\"-s\"")
    set(CGO_LDFLAGS "cgo LDFLAGS: ${LUA_LIB_PATH}/libluasandbox.dll ${LUA_LIB_PATH}/libluasb.dll -lm")
    set(CPACK_GENERATOR "ZIP")
elseif(APPLE)
    set(CPACK_GENERATOR "PackageMaker")
    set(LDFLAGS "-ldflags=\"-linkmode=external\"")
    set(CGO_LDFLAGS "cgo LDFLAGS: ${LUA_LIB_PATH}/libluasandbox.dylib ${LUA_LIB_PATH}/libluasb.dylib -ldl -lm")
    set(LUA_SHARED_LIBRARY_SUFFIX ".so")
elseif(UNIX)
    set(CPACK_GENERATOR "TGZ")
    set(LDFLAGS "-ldflags=\"-s\"")
    if(NOT CMAKE_SYSTEM_NAME MATCHES "FreeBSD")
        set(LINK_DL "-ldl")
    endif()
    set(CGO_LDFLAGS "cgo LDFLAGS: -Wl,-rpath,$ORIGIN/../lib ${LUA_LIB_PATH}/libluasandbox.so ${LUA_LIB_PATH}/libluasb.so ${LINK_DL} -lm")

    find_program(DPKG_EXECUTABLE dpkg PATH_SUFFIXES bin)

    find_program(RPMBUILD_EXECUTABLE rpmbuild PATH_SUFFIXES bin)
    if (RPMBUILD_EXECUTABLE)
        set(CPACK_GENERATOR ${CPACK_GENERATOR} "RPM")
        set(CPACK_RPM_PACKAGE_LICENSE "MPLv2.0")
		# todo we should pull lua_sandbox out of the heka/build and packaging
		set(CPACK_RPM_PACKAGE_PROVIDES "libluasb.so.0()(64bit), libluasandbox.so.0()(64bit)")
		set(CPACK_RPM_EXCLUDE_FROM_AUTO_FILELIST_ADDITION /usr/share/man)
    endif()
endif()

include(CTest)
include(externals)
include(mocks)

execute_process(COMMAND "${GIT_EXECUTABLE}" submodule update --init --recursive
WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}"
RESULT_VARIABLE error_code
)
if(error_code)
    message(FATAL_ERROR "Failed to init Heka submodules")
endif()

configure_file("${CMAKE_SOURCE_DIR}/sandbox/lua/lua_sandbox.go.in" "${HEKA_PATH}/sandbox/lua/lua_sandbox.go" @ONLY)

if (EXISTS "${CMAKE_BINARY_DIR}/plugin_loader.go")
    configure_file("${CMAKE_BINARY_DIR}/plugin_loader.go" "${HEKA_PATH}/cmd/hekad/plugin_loader.go" COPYONLY)
endif()

add_custom_target(clean-heka
COMMAND ${CMAKE_COMMAND} -E remove_directory "${HEKA_PATH}"
COMMAND ${CMAKE_COMMAND} -E remove "${HEKA_EXE}" "${FLOOD_EXE}" "${SBMGR_EXE}" "${SBMGRLOAD_EXE}" "${INJECT_EXE}"
COMMAND ${CMAKE_COMMAND} ..
COMMENT "Resynchronizing the Go workspace with the Heka repository"
)

set(MESSAGE_PROTO_OUT "${CMAKE_SOURCE_DIR}/message/message.pb.go")
add_custom_command(
OUTPUT ${MESSAGE_PROTO_OUT}
COMMAND ${CMAKE_COMMAND} -DSRC_DIR="${CMAKE_SOURCE_DIR}" -DPROTOBUF_EXECUTABLE="${PROTOBUF_EXECUTABLE}" -P "${CMAKE_SOURCE_DIR}/cmake/message_proto.cmake"
DEPENDS "${CMAKE_SOURCE_DIR}/message/message.proto"
WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/message"
COMMENT "Built ${MESSAGE_PROTO_OUT}"
)

if(INCLUDE_SANDBOX)
set(COPY_SANDBOX COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/sandbox" "${HEKA_PATH}/sandbox")
endif()
add_custom_target(heka_source ALL
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/client" "${HEKA_PATH}/client"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/cmd" "${HEKA_PATH}/cmd"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/docs" "${HEKA_PATH}/docs"
COMMAND ${CMAKE_COMMAND} -E copy "${CMAKE_SOURCE_DIR}/CHANGES.txt" "${HEKA_PATH}/docs"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/examples" "${HEKA_PATH}/examples"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/message" "${HEKA_PATH}/message"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/pipeline" "${HEKA_PATH}/pipeline"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/plugins" "${HEKA_PATH}/plugins"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/logstreamer" "${HEKA_PATH}/logstreamer"
COMMAND ${CMAKE_COMMAND} -E copy_directory "${CMAKE_SOURCE_DIR}/ringbuf" "${HEKA_PATH}/ringbuf"
${COPY_SANDBOX}
DEPENDS ${SANDBOX_PACKAGE} GoPackages ${MESSAGE_PROTO_OUT}
)

add_custom_target(message_matcher_parser ALL
COMMAND ${GO_EXECUTABLE} tool yacc -l=false -o=message_matcher_parser.go message_matcher_parser.y
DEPENDS heka_source
WORKING_DIRECTORY "${HEKA_PATH}/message"
)
add_dependencies(mocks message_matcher_parser)

add_custom_target(hekad ALL
${GO_EXECUTABLE} install ${LDFLAGS} -tags=${TAGS} github.com/mozilla-services/heka/cmd/hekad
DEPENDS mocks)

if (INCLUDE_DOCUMENTATION)
    add_custom_target(docs
    COMMAND ${SPHINX_BUILD_EXECUTABLE} -b html -d build/doctrees source build/html
    COMMAND ${SPHINX_BUILD_EXECUTABLE} -b man -d build/doctrees source build/man
    WORKING_DIRECTORY "${HEKA_PATH}/docs"
    DEPENDS heka_source
    COMMENT "Built Heka user/developer documentation")

    install(CODE "execute_process(COMMAND ${CMAKE_BUILD_TOOL} docs)") # always force the docs to rebuild before installing/packaging
    install(DIRECTORY "${HEKA_PATH}/docs/build/man/" DESTINATION share/man/man1 FILES_MATCHING PATTERN "*.1")
    install(DIRECTORY "${HEKA_PATH}/docs/build/man/" DESTINATION share/man/man5 FILES_MATCHING PATTERN "*.5")
endif()

if (MINGW AND INCLUDE_SANDBOX)
    add_custom_command(TARGET heka_source POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}"
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}/pipeline"
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}/plugins"
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}/sandbox/lua"
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}/sandbox/plugins"
    COMMAND ${CMAKE_COMMAND} -E copy_directory "${PROJECT_PATH}/lib/" "${HEKA_PATH}/cmd/hekad"
    COMMENT "Install dll's for the mock generation and unit tests")
    install(DIRECTORY "${PROJECT_PATH}/lib/" DESTINATION bin PATTERN "*.dll" PATTERN "luasandbox" EXCLUDE PATTERN "luasandbox/*" EXCLUDE)
else()
    install(DIRECTORY "${PROJECT_PATH}/lib/" DESTINATION lib PATTERN "*${CMAKE_SHARED_LIBRARY_SUFFIX}" PATTERN "luasandbox" EXCLUDE PATTERN "luasandbox/*" EXCLUDE)
endif()

install(PROGRAMS "${HEKA_EXE}" DESTINATION bin)

add_custom_target(flood ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-flood
DEPENDS hekad
WORKING_DIRECTORY ${CMAKE_SOURCE_DIR})

install(PROGRAMS "${FLOOD_EXE}" DESTINATION bin)

add_custom_target(inject ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-inject
DEPENDS hekad
WORKING_DIRECTORY ${CMAKE_SOURCE_DIR})

install(PROGRAMS "${INJECT_EXE}" DESTINATION bin)

add_custom_target(logstreamer ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-logstreamer
DEPENDS hekad
WORKING_DIRECTORY ${CMAKE_SOURCE_DIR})

install(PROGRAMS "${LOGSTREAMER_EXE}" DESTINATION bin)

add_custom_target(heka-cat ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-cat
DEPENDS hekad
WORKING_DIRECTORY ${CMAKE_SOURCE_DIR})

install(PROGRAMS "${HEKA_CAT_EXE}" DESTINATION bin)

add_custom_target(sbmgr ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-sbmgr
DEPENDS hekad)

install(PROGRAMS "${SBMGR_EXE}" DESTINATION bin)

add_custom_target(sbmgrload ALL
${GO_EXECUTABLE} install ${LDFLAGS} github.com/mozilla-services/heka/cmd/heka-sbmgrload
DEPENDS hekad)

## DEBIAN SPECIFIC THINGS HERE
set(CPACK_PROJECT_CONFIG_FILE "${CMAKE_SOURCE_DIR}/CPackConfig.cmake")
if (UNIX AND DPKG_EXECUTABLE)
    get_filename_component(CMAKE_BIN_DIR ${CMAKE_COMMAND} PATH)
    set(CPACK_COMMAND ${CMAKE_BIN_DIR}/cpack)

    execute_process(COMMAND "${DPKG_EXECUTABLE}" --print-architecture
        OUTPUT_VARIABLE CPACK_DEBIAN_PACKAGE_ARCHITECTURE OUTPUT_STRIP_TRAILING_WHITESPACE)

    add_custom_target(deb
        COMMAND ${CMAKE_COMMAND} -E cmake_echo_color --switch="$(COLOR)" --cyan "Run CPack packaging tool..."
        COMMAND ${CPACK_COMMAND} -G DEB
        DEPENDS hekad
        COMMENT "Custom deb target")

    add_subdirectory(debian)
endif()

# MOVING INCLUSION OF CPACK DOWN HERE SO IT ACTUALLY GETS THE VARIABLES WE SET
include(CPack)

add_test(cmd/hekad ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/cmd/hekad)
add_test(message ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/message)
add_test(pipeline ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/pipeline)
add_test(plugins ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins)
add_test(plugins/amqp ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/amqp)
add_test(plugins/dasher ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/dasher)
add_test(plugins/elasticsearch ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/elasticsearch)
add_test(plugins/file ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/file)
if (INCLUDE_GEOIP)
    add_test(plugins/geoip  ${GO_EXECUTABLE} test ${LDFLAGS} -tags=${TAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/geoip)
endif()
add_test(plugins/graphite ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/graphite)
add_test(plugins/http ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/http)
add_test(plugins/irc ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/irc)
add_test(plugins/kafka ${GO_EXECUTABLE} test -timeout 15s  ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/kafka)
add_test(plugins/logstreamer ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/logstreamer)
add_test(plugins/nagios ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/nagios)
add_test(plugins/payload ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/payload)
add_test(plugins/process ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/process)
add_test(plugins/smtp ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/smtp)
add_test(plugins/statsd ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/statsd)
add_test(plugins/tcp ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/tcp)
add_test(plugins/udp ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/plugins/udp)
add_test(logstreamer ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/logstreamer)
add_test(client ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/client)
if(INCLUDE_SANDBOX)
    add_test(sandbox_move_modules cmake -E copy_directory ${CMAKE_BINARY_DIR}/heka/lib/luasandbox/modules ${CMAKE_BINARY_DIR}/heka/src/github.com/mozilla-services/heka/sandbox/lua/modules)
    add_test(sandbox ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/sandbox/lua)
    add_test(sandbox_plugins ${GO_EXECUTABLE} test ${LDFLAGS} ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka/sandbox/plugins)
endif()
if (INCLUDE_MOZSVC)
    add_test(mozsvc ${GO_EXECUTABLE} test ${BENCHMARK_FLAG} ${COVERAGE_FLAG} github.com/mozilla-services/heka-mozsvc-plugins)
endif()
install(FILES "${HEKA_PATH}/cmd/heka-sbmgr/sbmgr.toml" "${HEKA_PATH}/cmd/heka-sbmgr/hekad.toml.sbmgr" DESTINATION bin)
if(INCLUDE_SANDBOX)
install(FILES "${HEKA_PATH}/sandbox/lua/testsupport/hekabench_cbuf_counter.lua" DESTINATION bin)
endif()
install(FILES "${CMAKE_SOURCE_DIR}/LICENSE.txt" DESTINATION "share/${CMAKE_PROJECT_NAME}")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/dasher" DESTINATION "share/${CMAKE_PROJECT_NAME}")
install(DIRECTORY "${PROJECT_PATH}/lib/luasandbox/modules/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_modules")
install(DIRECTORY "${PROJECT_PATH}/lib/luasandbox/io_modules/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_io_modules")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/examples/conf/" DESTINATION "share/${CMAKE_PROJECT_NAME}/examples")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/sandbox/lua/modules/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_modules")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/sandbox/lua/decoders/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_decoders")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/sandbox/lua/filters/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_filters")
install(DIRECTORY "${CMAKE_SOURCE_DIR}/sandbox/lua/encoders/" DESTINATION "share/${CMAKE_PROJECT_NAME}/lua_encoders")

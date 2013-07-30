# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

# The module defines the following variables:
#   PROTOBUF_FOUND - true if the Protobuf was found
#   PROTOBUF_EXECUTABLE - path to the executable
#   PROTOBUF_VERSION - Protobuf version number
# Example usage:
#   find_package(Protobuf 2.3 REQUIRED)


find_program(PROTOBUF_EXECUTABLE protoc PATH_SUFFIXES bin)
if (PROTOBUF_EXECUTABLE)
    execute_process(COMMAND ${PROTOBUF_EXECUTABLE} --version OUTPUT_VARIABLE PROTOBUF_VERSION_OUTPUT OUTPUT_STRIP_TRAILING_WHITESPACE)
    if(PROTOBUF_VERSION_OUTPUT MATCHES "libprotoc ([0-9]+\\.[0-9]+\\.[0-9]+)")
        set(PROTOBUF_VERSION ${CMAKE_MATCH_1})
    endif()
endif()
mark_as_advanced(PROTOBUF_EXECUTABLE)

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(Protobuf REQUIRED_VARS PROTOBUF_EXECUTABLE PROTOBUF_VERSION VERSION_VAR PROTOBUF_VERSION)

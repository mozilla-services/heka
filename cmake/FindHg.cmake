# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

# The module defines the following variables:
#   HG_FOUND - true if the Hg was found
#   HG_EXECUTABLE - path to the executable
#   HG_VERSION - Hg version number
# Example usage:
#   find_package(Hg 2.0 REQUIRED)


find_program(HG_EXECUTABLE hg PATHS ENV HG_ROOT HG_PATH PATH_SUFFIXES bin)
if (HG_EXECUTABLE)
    execute_process(COMMAND ${HG_EXECUTABLE} version OUTPUT_VARIABLE HG_VERSION_OUTPUT OUTPUT_STRIP_TRAILING_WHITESPACE)
    if(HG_VERSION_OUTPUT MATCHES "\(version ([^)]+)\)")
        set(HG_VERSION ${CMAKE_MATCH_1})
    endif()
endif()
mark_as_advanced(HG_EXECUTABLE)

include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(Hg REQUIRED_VARS HG_EXECUTABLE HG_VERSION VERSION_VAR HG_VERSION)

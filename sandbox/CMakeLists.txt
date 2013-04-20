# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

execute_process(COMMAND go env GOGCCFLAGS OUTPUT_VARIABLE GOGCCFLAGS OUTPUT_STRIP_TRAILING_WHITESPACE)
add_library(sandbox SHARED lua/lua_sandbox.c lua/lua_sandbox_private.c lua/lua_circular_buffer.c)

if(CMAKE_HOST_UNIX)
	set(CMAKE_C_FLAGS "${GOGCCFLAGS} -shared -std=gnu99")
	if (APPLE)
		set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -flat_namespace -undefined dynamic_lookup")
   endif(APPLE)

	target_link_libraries(sandbox ${CMAKE_BINARY_DIR}/external/lib/liblua.a ${MATH_LIBRARY})
endif(CMAKE_HOST_UNIX)

install(TARGETS sandbox DESTINATION bin)

add_custom_target(sandbox_install 
   ALL
   "${CMAKE_COMMAND}" 
   -D CMAKE_INSTALL_PREFIX:string=${CMAKE_SOURCE_DIR}
   -P "${CMAKE_CURRENT_BINARY_DIR}/cmake_install.cmake" 
   DEPENDS sandbox) 


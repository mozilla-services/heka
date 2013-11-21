# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

include(ExternalProject)

get_filename_component(GIT_PATH ${GIT_EXECUTABLE} PATH)
find_program(PATCH_EXECUTABLE patch HINTS "${GIT_PATH}" "${GIT_PATH}/../bin")
if (NOT PATCH_EXECUTABLE)
   message(FATAL_ERROR "patch not found")
endif()

set_property(DIRECTORY PROPERTY EP_BASE "${CMAKE_BINARY_DIR}/ep_base")

externalproject_add(
    lpeg-0_12
    URL http://www.inf.puc-rio.br/~roberto/lpeg/lpeg-0.12.tar.gz
    URL_MD5 4abb3c28cd8b6565c6a65e88f06c9162
    PATCH_COMMAND ${PATCH_EXECUTABLE} -p1 < ${CMAKE_CURRENT_LIST_DIR}/lpeg-0_12.patch
    CONFIGURE_COMMAND ""
    BUILD_COMMAND ""
    INSTALL_COMMAND ""
)

externalproject_add(
    lua-cjson-2_1_0
    URL http://www.kyne.com.au/~mark/software/download/lua-cjson-2.1.0.tar.gz
    URL_MD5 24f270663e9f6ca8ba2a02cef19f7963
    PATCH_COMMAND ${PATCH_EXECUTABLE} -p1 < ${CMAKE_CURRENT_LIST_DIR}/lua-cjson-2_1_0.patch
    CONFIGURE_COMMAND ""
    BUILD_COMMAND ""
    INSTALL_COMMAND ""
)

externalproject_add(
    lua-5_1_5
    URL http://www.lua.org/ftp/lua-5.1.5.tar.gz
    URL_MD5 2e115fe26e435e33b0d5c022e4490567
    PATCH_COMMAND ${PATCH_EXECUTABLE} -p1 < ${CMAKE_CURRENT_LIST_DIR}/lua-5_1_5.patch
    CMAKE_ARGS -DCMAKE_INSTALL_PREFIX=${PROJECT_PATH} -DADDRESS_MODEL=${ADDRESS_MODEL} --no-warn-unused-cli
    INSTALL_DIRECTORY ${PROJECT_PATH}
)
add_dependencies(lua-5_1_5 lpeg-0_12 lua-cjson-2_1_0)

if ("$ENV{GOPATH}" STREQUAL "")
   message(FATAL_ERROR "No GOPATH environment variable has been set. $ENV{GOPATH}")
endif()

add_custom_target(GoPackages ALL)

function(git_clone url tag)
    string(REGEX REPLACE ".*/" "" _name ${url})
    string(REGEX REPLACE "https?://" "" _path ${url})
    externalproject_add(
        ${_name}
        GIT_REPOSITORY ${url}
        GIT_TAG ${tag}
        SOURCE_DIR "${PROJECT_PATH}/src/${_path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_COMMAND "" # comment out to enable updates
    )
    add_dependencies(GoPackages ${_name})
endfunction(git_clone)

function(hg_clone url tag)
    string(REGEX REPLACE ".*/" "" _name ${url})
    string(REGEX REPLACE "https?://" "" _path ${url})
    externalproject_add(
        ${_name}
        HG_REPOSITORY ${url}
        HG_TAG ${tag}
        SOURCE_DIR "${PROJECT_PATH}/src/${_path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_COMMAND "" # comment out to enable updates
    )
    add_dependencies(GoPackages ${_name})
endfunction(hg_clone)

function(add_external_plugin vcs url tag)
    string(REGEX REPLACE "https?://" "" _path ${url})
    if ("${vcs}" STREQUAL "git")
       git_clone(${url} ${tag})
    elseif("${vcs}" STREQUAL "hg")
       hg_clone(${url} ${tag})
    else()
        message(FATAL_ERROR "Unknown version control system ${vcs}")
    endif()

    set(_packages ${_path})
    foreach(_subpath ${ARGN})
        set(_packages ${_packages} "${_path}/${_subpath}")
    endforeach()
    set(PLUGIN_LOADER ${PLUGIN_LOADER} ${_packages} PARENT_SCOPE)
endfunction(add_external_plugin)

git_clone(https://code.google.com/p/gomock master)
add_custom_command(TARGET gomock POST_BUILD
COMMAND ${GO_EXECUTABLE} install code.google.com/p/gomock/mockgen)
git_clone(https://github.com/bitly/go-simplejson ec501b3f691bcc79d97caf8fdf28bcf136efdab8)
git_clone(https://github.com/rafrombrc/whisper-go 89e9ba3b5c6a10d8ac43bd1a25371f3e6118c37f)
git_clone(https://github.com/rafrombrc/go-notify e3ddb616eea90d4e87dff8513c251ff514678406)
git_clone(https://github.com/bbangert/toml a2063ce2e5cf10e54ab24075840593d60f59b611)
git_clone(https://github.com/streadway/amqp 171c24a86dfdd0ab079c4077500fd6bf59b6b00b)
git_clone(https://github.com/feyeleanor/raw 724aedf6e1a5d8971aafec384b6bde3d5608fba4)
git_clone(https://github.com/feyeleanor/slices bb44bb2e4817fe71ba7082d351fd582e7d40e3ea)
add_dependencies(slices raw)
git_clone(https://github.com/feyeleanor/sets 6c54cb57ea406ff6354256a4847e37298194478f)
add_dependencies(sets slices)
git_clone(https://github.com/crowdmob/goamz 7168305bd984b32bef7157a672e2460d0b0bba2f)
git_clone(https://github.com/rafrombrc/gospec master)
git_clone(https://github.com/crankycoder/g2s master)
git_clone(https://github.com/crankycoder/xmlpath master)

if (INCLUDE_MOZSVC)
    add_external_plugin(git https://github.com/mozilla-services/heka-mozsvc-plugins v0.4.1)
endif()

if (INCLUDE_DOCUMENTATION)
    git_clone(https://github.com/mozilla-services/heka-docs dev)

    add_custom_command(TARGET docs POST_BUILD
    COMMAND ${SPHINX_BUILD_EXECUTABLE} -b html -d build/doctrees source build/html
    WORKING_DIRECTORY "${HEKA_PATH}/../heka-docs"
    COMMENT "Built Heka architecture documentation")
endif()

hg_clone(https://code.google.com/p/go-uuid default)
hg_clone(https://code.google.com/p/goprotobuf default)
add_custom_command(TARGET goprotobuf POST_BUILD
COMMAND ${GO_EXECUTABLE} install code.google.com/p/goprotobuf/protoc-gen-go)

include(plugin_loader OPTIONAL)

if (PLUGIN_LOADER)
    set(_PLUGIN_LOADER_OUTPUT "package main\n\nimport (")
    list(SORT PLUGIN_LOADER)
    foreach(PLUGIN IN ITEMS ${PLUGIN_LOADER})
         set(_PLUGIN_LOADER_OUTPUT "${_PLUGIN_LOADER_OUTPUT}\n\t _ \"${PLUGIN}\"")
    endforeach()
    set(_PLUGIN_LOADER_OUTPUT "${_PLUGIN_LOADER_OUTPUT}\n)\n")
    file(WRITE "${CMAKE_BINARY_DIR}/plugin_loader.go" ${_PLUGIN_LOADER_OUTPUT})
endif()

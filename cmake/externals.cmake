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

if(INCLUDE_SANDBOX)
    set(PLUGIN_LOADER ${PLUGIN_LOADER} "github.com/mozilla-services/heka/sandbox/plugins")
    set(SANDBOX_PACKAGE "luasandbox-0_1_0")
    set(SANDBOX_ARGS -DCMAKE_BUILD_TYPE=${CMAKE_BUILD_TYPE} -DCMAKE_INSTALL_PREFIX=${PROJECT_PATH} -DADDRESS_MODEL=${ADDRESS_MODEL} -DLUA_JIT=off --no-warn-unused-cli)
    externalproject_add(
        ${SANDBOX_PACKAGE}
        GIT_REPOSITORY https://github.com/mozilla-services/lua_sandbox.git
        GIT_TAG 6467325551b14f44b2cc0729dc2e5b8709d4ef87
        CMAKE_ARGS ${SANDBOX_ARGS}
        INSTALL_DIR ${PROJECT_PATH}
    )
endif()

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

git_clone(https://code.google.com/p/gomock ae48011f41cd)
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
git_clone(https://github.com/rafrombrc/gospec 2e46585948f47047b0c217d00fa24bbc4e370e6b)
git_clone(https://github.com/crankycoder/g2s 2594f7a035ed881bb10618bc5dc4440ef35c6a29)
git_clone(https://github.com/crankycoder/xmlpath 670b185b686fd11aa115291fb2f6dc3ed7ebb488)

if (INCLUDE_MOZSVC)
    add_external_plugin(git https://github.com/mozilla-services/heka-mozsvc-plugins 6fe574dbd32a21f5d5583608a9d2339925edd2a7)
endif()

if (INCLUDE_DOCUMENTATION)
    git_clone(https://github.com/mozilla-services/heka-docs cb4a1610579c02bb25a8c0aaf835b05c3214d532)

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

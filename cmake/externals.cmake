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
    set(SANDBOX_PACKAGE "lua_sandbox")
    set(SANDBOX_ARGS -DCMAKE_BUILD_TYPE=${CMAKE_BUILD_TYPE} -DCMAKE_INSTALL_PREFIX=${PROJECT_PATH} -DADDRESS_MODEL=${ADDRESS_MODEL} -DLUA_JIT=off --no-warn-unused-cli)
    externalproject_add(
        ${SANDBOX_PACKAGE}
        GIT_REPOSITORY https://github.com/mozilla-services/lua_sandbox.git
        GIT_TAG ccc0625d30f8cf6453abd79e92c7a9bac845f6be
        CMAKE_ARGS ${SANDBOX_ARGS}
        INSTALL_DIR ${PROJECT_PATH}
    )
endif()

if ("$ENV{GOPATH}" STREQUAL "")
   message(FATAL_ERROR "No GOPATH environment variable has been set. $ENV{GOPATH}")
endif()

add_custom_target(GoPackages ALL)

function(parse_url url)
    string(REGEX REPLACE ".*/" "" _name ${url})
    set(name ${_name} PARENT_SCOPE)

    string(REGEX REPLACE "^https?://([A-Za-z0-9$-._~!:;=]+@)?" "" _path ${url})

    set(path ${_path} PARENT_SCOPE)
endfunction(parse_url)

function(git_clone url tag)
    parse_url(${url})
    externalproject_add(
        ${name}
        GIT_REPOSITORY ${url}
        GIT_TAG ${tag}
        SOURCE_DIR "${PROJECT_PATH}/src/${path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_COMMAND "" # comment out to enable updates
    )
    add_dependencies(GoPackages ${name})
endfunction(git_clone)

function(hg_clone url tag)
    parse_url(${url})
    externalproject_add(
        ${name}
        HG_REPOSITORY ${url}
        HG_TAG ${tag}
        SOURCE_DIR "${PROJECT_PATH}/src/${path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_COMMAND "" # comment out to enable updates
    )
    add_dependencies(GoPackages ${name})
endfunction(hg_clone)

function(svn_clone url tag)
    parse_url(${url})
    externalproject_add(
        ${name}
        SVN_REPOSITORY ${url}
        SVN_REVISION ${tag}
        SOURCE_DIR "${PROJECT_PATH}/src/${path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_COMMAND "" # comment out to enable updates
    )
    add_dependencies(GoPackages ${name})
endfunction(svn_clone )

function(local_clone url)
    parse_url(${url})
    externalproject_add(
        ${name}
        URL ${CMAKE_SOURCE_DIR}/externals/${name}
        SOURCE_DIR "${PROJECT_PATH}/src/${path}"
        BUILD_COMMAND ""
        CONFIGURE_COMMAND ""
        INSTALL_COMMAND ""
        UPDATE_ALWAYS true
    )
    add_dependencies(GoPackages ${name})
endfunction(local_clone)

function(add_external_plugin vcs url tag)
    parse_url(${url})
    if  ("${tag}" STREQUAL ":local")
       local_clone(${url})
    else()
        if ("${vcs}" STREQUAL "git")
           git_clone(${url} ${tag})
        elseif("${vcs}" STREQUAL "hg")
           hg_clone(${url} ${tag})
        elseif("${vcs}" STREQUAL "svn")
           svn_clone(${url} ${tag})
        else()
           message(FATAL_ERROR "Unknown version control system ${vcs}")
        endif()
    endif()

    set(_packages ${path})
    foreach(_subpath ${ARGN})
        set(_packages ${_packages} "${path}/${_subpath}")
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
git_clone(https://github.com/crowdmob/goamz e9a919b6da95151fc77b1b7bb3e78a8a68379aa1)
git_clone(https://github.com/rafrombrc/gospec 2e46585948f47047b0c217d00fa24bbc4e370e6b)
git_clone(https://github.com/crankycoder/g2s 2594f7a035ed881bb10618bc5dc4440ef35c6a29)
git_clone(https://github.com/crankycoder/xmlpath 670b185b686fd11aa115291fb2f6dc3ed7ebb488)

if (INCLUDE_GEOIP)
    add_external_plugin(git https://github.com/abh/geoip da130741c8ed2052f5f455d56e552f2e997e1ce9)
endif()

if (INCLUDE_MOZSVC)
    add_external_plugin(git https://github.com/mozilla-services/heka-mozsvc-plugins 9e454bebb5085e25fc50f32556502141503b69e4)
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

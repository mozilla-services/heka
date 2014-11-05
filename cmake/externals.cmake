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
        GIT_TAG 6eb7a907d8f4777418463dc5b56d237f1c845a67
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

    # For details of the URI parsing see: http://tools.ietf.org/html/rfc3986#appendix-A
    string(REGEX REPLACE "^[a-zA-Z][-+.a-zA-Z0-9]+://" "" _path ${url}) # strip the scheme
    string(REGEX REPLACE "^[A-Za-z0-9$-._~!:;=]+@" "" _path ${_path}) # strip the userinfo
    string(REGEX REPLACE "^([^:/]+):[0-9]+/" "\\1/" _path ${_path}) # strip the port
    string(REGEX REPLACE "^([^:/]+):/?" "\\1/" _path ${_path}) # strip the colon separator and make sure we have a slash
    string(REGEX REPLACE "#.*$" "" _path ${_path}) # strip the revision

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

    set(ignore_root FALSE)
    foreach(_subpath ${ARGN})
        if ("${_subpath}" STREQUAL "__ignore_root")
            set(ignore_root TRUE)
        else()
            set(_packages ${_packages} "${path}/${_subpath}")
        endif()
    endforeach()

    if (NOT ${ignore_root})
        set(_packages ${path})
    endif()
    set(PLUGIN_LOADER ${PLUGIN_LOADER} ${_packages} PARENT_SCOPE)
endfunction(add_external_plugin)

git_clone(https://github.com/rafrombrc/gomock c922279faf77f29ce5781e96eb0711837fcb477c)
add_custom_command(TARGET gomock POST_BUILD
COMMAND ${GO_EXECUTABLE} install github.com/rafrombrc/gomock/mockgen)
git_clone(https://github.com/bitly/go-simplejson ec501b3f691bcc79d97caf8fdf28bcf136efdab8)
git_clone(https://github.com/rafrombrc/whisper-go 89e9ba3b5c6a10d8ac43bd1a25371f3e6118c37f)
git_clone(https://github.com/rafrombrc/go-notify e3ddb616eea90d4e87dff8513c251ff514678406)
git_clone(https://github.com/bbangert/toml a2063ce2e5cf10e54ab24075840593d60f59b611)
git_clone(https://github.com/streadway/amqp 7d6d1802c7710be39564a287f860360c6328f956)
git_clone(https://github.com/feyeleanor/raw 724aedf6e1a5d8971aafec384b6bde3d5608fba4)
git_clone(https://github.com/feyeleanor/slices bb44bb2e4817fe71ba7082d351fd582e7d40e3ea)
add_dependencies(slices raw)
git_clone(https://github.com/feyeleanor/sets 6c54cb57ea406ff6354256a4847e37298194478f)
add_dependencies(sets slices)
git_clone(https://github.com/crowdmob/goamz e9a919b6da95151fc77b1b7bb3e78a8a68379aa1)
git_clone(https://github.com/rafrombrc/gospec 2e46585948f47047b0c217d00fa24bbc4e370e6b)
git_clone(https://github.com/crankycoder/g2s 2594f7a035ed881bb10618bc5dc4440ef35c6a29)
git_clone(https://github.com/crankycoder/xmlpath 670b185b686fd11aa115291fb2f6dc3ed7ebb488)
git_clone(https://github.com/thoj/go-ircevent 90dc7f966b95d133f1c65531c6959b52effd5e40)

if (INCLUDE_GEOIP)
    add_external_plugin(git https://github.com/abh/geoip da130741c8ed2052f5f455d56e552f2e997e1ce9)
endif()

if (INCLUDE_DOCKER_PLUGINS)
    git_clone(https://github.com/rafrombrc/go-dockerclient 253de7054ca5defe718269e17732e24cdadc3d21)
endif()

if (INCLUDE_MOZSVC)
    add_external_plugin(git https://github.com/mozilla-services/heka-mozsvc-plugins 91278658b5d52bd45b0b74d54e478a230c0ef0c4)
    git_clone(https://github.com/getsentry/raven-go 0cc1491d9d27b258a9b4f0238908cb0d51bd6c9b)
    add_dependencies(heka-mozsvc-plugins raven-go)
endif()

if (INCLUDE_DOCUMENTATION)
    git_clone(https://github.com/mozilla-services/heka-docs cb4a1610579c02bb25a8c0aaf835b05c3214d532)

    add_custom_command(TARGET docs POST_BUILD
    COMMAND ${SPHINX_BUILD_EXECUTABLE} -b html -d build/doctrees source build/html
    WORKING_DIRECTORY "${HEKA_PATH}/../heka-docs"
    COMMENT "Built Heka architecture documentation")
endif()

hg_clone(https://code.google.com/p/go-uuid default)
git_clone(https://code.google.com/p/gogoprotobuf 7008a93e68bf)
add_custom_command(TARGET gogoprotobuf POST_BUILD
COMMAND ${GO_EXECUTABLE} install code.google.com/p/gogoprotobuf/protoc-gen-gogo)

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

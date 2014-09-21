# See http://www.cmake.org/Wiki/CMake:CPackPackageGenerators#Overall_usage_.28common_to_all_generators.29

if(CPACK_GENERATOR MATCHES "DEB")
    # When the DEB-generator runs, we want him to run our install-script
    set(CPACK_INSTALL_SCRIPT ${CPACK_DEBIAN_INSTALL_SCRIPT})
endif()

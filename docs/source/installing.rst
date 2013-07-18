.. _installing:

==========
Installing
==========

.. _from_binaries:

Binaries
========

`hekad` `releases are available on the Github project releases page
<https://github.com/mozilla-services/heka/releases>`_.
Binaries are available for Linux and OSX, with packages for Debian and
RPM based distributions.

.. _from_source:

From Source
===========

`hekad` requires a Go work environment to be setup for the binary to be
built; this task is automated by the build process. The build script will
override the Go environment for the shell window it is executed in. This creates
an isolated environment that is intended specifically for building and 
developing Heka.  The build script should be be run every time a new shell is 
opened for Heka development to ensure the correct dependencies are found and 
being used. To create a working `hekad` binary for your platform you'll need to
install some prerequisites. Many of these are standard on modern Unix 
distributions and all are available for installation on Windows systems.

Prerequisites (all systems):

- CMake 2.8 or greater http://www.cmake.org/cmake/resources/software.html
- Git http://code.google.com/p/msysgit/downloads/list
- Go 1.1 or greater (1.1.1 recommended) http://code.google.com/p/go/downloads/list
- Mercurial http://mercurial.selenic.com/downloads/
- Protobuf 2.3 or greater http://code.google.com/p/protobuf/downloads/list
- Sphinx (optional - used to generate the documentation) http://sphinx-doc.org/

Prerequisites (Unix):

- make
- gcc
- patch
- dpkg (optional)
- rpmbuild (optional)
- packagemaker (optional)

Prerequisites (Windows):

- MinGW http://sourceforge.net/projects/tdm-gcc/


1. Check out the `heka`_ repository:

    .. code-block:: bash

        git clone https://github.com/mozilla-services/heka

2. Run `build` in the heka directory

    .. code-block:: bash

        cd heka
        . build.sh # Unix (note the dot: this file must be sourced to properly setup the environment)
        build.bat  # Windows

You will now have a `hekad` binary in the `build/heka/bin` directory.

3. (Optional) Run the tests to ensure a functioning `hekad`:

    .. code-block:: bash

        make test         # Unix
        mingw32-make test # Windows

.. _build_include_externals:

Building `hekad` with External Plugins
======================================

It is possible to extend `hekad` by writing input, decoder, filter, or output
plugins in Go (see :ref:`plugins`). Because Go only supports static linking of
Go code, your plugins must be included with and registered into Heka at
compile time. The build process supports this through the use of an optional 
cmake file `{heka root}/cmake/plugin_loader.cmake`.  A cmake function has been
provided `add_external_plugin` taking the repository type (git, hg, or svn), 
repository URL, and the repository tag to fetch.

    .. code-block:: txt

    add_external_plugin(git https://github.com/mozilla-services/heka-mozsvc-plugins dev)

The preceeding entry clones the `heka-mozsvc-plugins` git repository into the Go
work environment, checks out the dev branch, and imports the package into 
`hekad` when `make` is run. By adding an `init() function <http://golang.org/doc/effective_go.html#init>`_ 
in your package you can make calls into `pipeline.RegisterPlugin` to register 
your plugins with Heka's configuration system.

.. _build_pkgs:

Creating Packages
=================

Installing packages on a system is generally the easiest way to deploy
`hekad`. These packages can be easily created after following the above
:ref:`From Source <from_source>` directions:

1. Run `make package` to build the appropriate package(s) for the current
system:

    .. code-block:: bash

        make package # Unix
        mingw32-make package # Windows

The packages will be created in the build directory.

.. note::

    You will need `rpmbuild` installed to build the rpms.

    .. seealso:: `Setting up an rpm-build environment <http://wiki.centos.org/HowTos/SetupRpmBuildEnvironment>`_

.. _installing:

==========
Installing
==========

.. _from_binaries:

Binaries
========

`hekad` `releases are available on the Mozilla Services website
<https://docs.services.mozilla.com/_static/binaries/hekad-0.2/>`_.
Binaries are available for Linux and OSX, with packages for Debian and
RPM based distributions.

.. _from_source:

From Source (*nix)
==================

`hekad` requires a Go work environment to be setup for the binary to be
built. This task has been automated in the `heka build`_ repository. To
create a working `hekad` binary for your platform you'll need to
install some prerequisites. Many of these are standard on modern linux
distributions and OSX.

Prerequisites:

- cmake 2.8 or greater
- make
- gcc
- g++
- git
- go 1.1 or greater (1.1.1 recommended)
- python 2.6 or greater
- patch
- mercurial

1. Check out the `heka build`_ repository:

    .. code-block:: bash

        git clone https://github.com/mozilla-services/heka-build.git

2. Run `make` in the heka-build directory (builds the current release (master
   branch); if you have go installed in a non-standard location, you may need
   to set the GOROOT environment variable):

    .. code-block:: bash

        cd heka-build
        make

You will now have a `hekad` binary in the `heka-build/bin` directory.

3. (Optional) Run the tests to ensure a functioning `hekad`:

    .. code-block:: bash

        make test

4. (Optional) If you want to build the latest code in development, run `make
   dev` to switch to the dev branch and then run `make`. If you need to revert
   back to the master branch at some point run `make undev`.


From Source (Windows)
=====================

Prerequisites (manual setup):

- Go 1.1+ (1.1.1 recommended) http://code.google.com/p/go/downloads/list
- Cmake 2.8+ http://www.cmake.org/cmake/resources/software.html
- Git http://code.google.com/p/msysgit/downloads/list
- Mercurial http://mercurial.selenic.com/downloads/
- MinGW http://sourceforge.net/projects/tdm-gcc/

1. From a Git shell check out the `heka build`_ repository:

    .. code-block:: bash

        git clone https://github.com/mozilla-services/heka-build.git

2. From a MinGW shell run `build.bat` in the heka-build directory:

    .. code-block:: bash

        cd heka-build
        build

3. (Optional) Run the tests to ensure a functioning `hekad`:

    .. code-block:: bash

        mingw32-make test

You will now have a `hekad` binary in the `release/heka-0_2_0_w(32|64)/bin` directory.

.. _build_include_externals:

Building `hekad` with External Plugins
======================================

It is possible to extend `hekad` by writing input, decoder, filter, or output
plugins in Go (see :ref:`plugins`). Because Go only supports static linking of
Go code, your plugins must be included with and registered into Heka at
compile time. `heka build`_ supports the use of a `{heka-build-
root}/etc/plugin_packages.json` file to specify which packages you'd like to
include in your build. The JSON should be an object with a single
`plugin_packages` attribute, with the value an array of package paths. For
example:

    .. code-block:: json

        {"plugin_packages": ["github.com/mozilla-services/heka-mozsvc-plugins"]}

would cause the `github.com/mozilla-services/heka-mozsvc-plugins` package to
be imported into `hekad` when you run `make`. By adding an `init() function
<http://golang.org/doc/effective_go.html#init>`_ in your package you can make
calls into `pipeline.RegisterPlugin` to register your plugins with Heka's
configuration system.

.. _build_rpm_deb_pkgs:

Creating RPM/Deb Packages
=========================

Installing packages on a system is generally the easiest way to deploy
`hekad`. These packages can be easily created after following the above
:ref:`From Source <from_source>` directions:

1. Install fpm:

    .. code-block:: bash

        gem install fpm

2. Run `make debs` (or `rpms`) to build the appropriate package (in the
`heka-build` directory):

    .. code-block:: bash

        make debs

The packages will be in the `debs` or `rpms` directory.

.. note::

    You will need `rpmbuild` installed to build the rpms.

    .. seealso:: `Setting up an rpm-build environment <http://wiki.centos.org/HowTos/SetupRpmBuildEnvironment>`_

.. _heka build: https://github.com/mozilla-services/heka-build

.. _installing:

==========
Installing
==========

.. _from_source:

From Source
===========

`hekad` requires a Go work environment to be setup for the binary to be
built. This task has been automated in the `heka build`_ repository. To
create a working `hekad` binary for your platform you'll need to
install some prerequisites. Many of these are standard on modern linux
distributions and OSX.

Prerequisites:

- cmake 2.8+
- make
- gcc
- g++
- git
- python 2.6 or greater
- perl


1. Check out the `heka build`_ repository:

    .. code-block:: bash

        git clone https://github.com/mozilla-services/heka-build.git

2. Run `make` in the heka-build directory:

    .. code-block:: bash

        cd heka-build
        make

3. (Optional) Run the tests to ensure a functioning `hekad`:

    .. code-block:: bash

        make test

You will now have a `hekad` binary in the `heka-build/bin` directory.
Note that this will not work unless you also install the libsandbox
library that is also created.

.. note::

    Building hekad requires a specific Go tip version that has been
    verified to work. This will be checked out and built in the
    `heka-build` directory.

.. _build_include_externals:

Building `hekad` with External Plugins
======================================

It is possible to extend `hekad` by writing input, decoder, filter, or output
plugins in Go. Because Go only supports static linking of Go code, your
plugins must be included with and registered into Heka at compile time. `heka
build`_ supports the use of a `{heka-build-root}/etc/plugin_packages.json`
file to specify which packages you'd like to include in your build. The JSON
should be an object with a single `plugin_packages` attribute, with the value
an array of package paths. For example:

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

Build Instructions
==================

Simply run `build_docker.sh`. This generates 2 Docker images, `heka_base` and `heka`.
The first image contains the Heka source and the Heka build artifacts, and the second
contains a deb packaged Heka installation. Both `ENTRYPOINTS` are set to `hekad`,
all args passed to `docker run` will be passed to `hekad` as `CMD` args.

Dockerfiles
===========

Heka comes with a few Dockerfiles in the repository, and each one of them is used
to build Heka into a set of Docker images.

## $PROJECT_ROOT/Dockerfile

The Dockerfile in the root of the project is used to create base image for
building Heka from source. This is great for if you want to use Docker for
development or as a way to generate Heka binaries or packages.

## $PROJECT_ROOT/docker/Dockerfile

The Dockerfile in this directory, (without the `.final`), uses the base Heka
Docker image to build Heka, then generates a `deb` package of Heka, and creates
a new Docker image using `Dockerfile.final` with only Heka installed using the
`deb`.

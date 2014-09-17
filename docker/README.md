Build Instructions
==================

Note: The follwing instructions require root access, because docker requires running as
root. You will need to use `sudo`, or add the user running the script to a
Unix group called `docker`. Instructions to do so can be found on the docker website
here: https://docs.docker.com/installation/ubuntulinux/#giving-non-root-access

Simply run `build_docker.sh`. This generates 2 Docker images, `heka_base` and `heka`.
The first image contains the Heka source and the Heka build artifacts, and the second
contains a deb packaged Heka installation. Both `ENTRYPOINTS` are set to `hekad`,
all args passed to `docker run` will be passed to `hekad` as `CMD` args.

Usage
-----

Once you've run `build_docker.sh` you can simply use the image like so:

````
docker run --name heka -it -p 4352:4352 -v /host/path/to/config.toml:/etc/heka/config.toml mozilla/heka -config /etc/heka/config.toml
````

This will create a container with the name `heka` with the Dashboard port exposed
on the Docker host machine. The `-v` flag is used here to share your config file
with the container. Replace `/host/path/to/config.toml` with the path to your
config file. As you can see, the `-config` flag at the end is passed directly to
`hekad` inside the container.

Dockerfiles
===========

Heka comes with a few Dockerfiles in the repository, and each one of them is used
to build Heka into a set of Docker images.

## `$PROJECT_ROOT/Dockerfile`

The Dockerfile in the root of the project is used to create base image for
building Heka from source. This is great for if you want to use Docker for
development or as a way to generate Heka binaries or packages.

## `$PROJECT_ROOT/docker/Dockerfile`

The Dockerfile in this directory, (without the `.final`), uses the base Heka
Docker image to build Heka, then generates a `deb` package of Heka, and creates
a new Docker image using `Dockerfile.final` with only Heka installed using the
`deb`.


Example
-------

Take a look in the `example/` directory.

